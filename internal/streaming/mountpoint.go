package streaming

import (
	"fmt"
	"io"
	"log"
	"sync"
	"time"
)

// Mountpoint represents a streaming mount point with thread-safe operations
type Mountpoint struct {
	path         string
	stationID    int64
	name         string
	genre        string
	contentType  string
	bitrate      int

	// Source management
	sourceMu     sync.RWMutex
	source       io.ReadCloser
	sourceActive bool

	// Listener management
	listenersMu sync.RWMutex
	listeners   map[string]*Listener
	nextID      int

	// Metadata
	metadataMu      sync.RWMutex
	currentMetadata string

	// Ring buffer for burst-on-connect
	ringBufferMu   sync.RWMutex
	ringBuffer     [][]byte
	ringBufferSize int
	ringBufferPos  int

	// Init segment for fMP4 streams
	initSegmentMu sync.RWMutex
	initSegment   []byte

	// Control channels
	stopChan     chan struct{}
	metadataChan chan string

	logger *log.Logger
}

// Listener represents a connected client listening to a stream (simplified for browser-only)
type Listener struct {
	ID          string
	Chan        chan []byte
	ConnectedAt time.Time
	BytesSent   int64
}

// NewMountpoint creates a new mountpoint
func NewMountpoint(path string, stationID int64, name, genre, contentType string, bitrate int, logger *log.Logger) *Mountpoint {
	// Calculate ring buffer size for burst-on-connect
	// Target: Keep enough chunks for smooth playback on Yggdrasil
	ringBufferSize := 256 // Keep last 256 chunks (~1MB for Yggdrasil high-latency network)

	mp := &Mountpoint{
		path:           path,
		stationID:      stationID,
		name:           name,
		genre:          genre,
		contentType:    contentType,
		bitrate:        bitrate,
		listeners:      make(map[string]*Listener),
		ringBuffer:     make([][]byte, ringBufferSize),
		ringBufferSize: ringBufferSize,
		ringBufferPos:  0,
		stopChan:       make(chan struct{}),
		metadataChan:   make(chan string, 10),
		logger:         logger,
	}

	// Start metadata processor
	go mp.processMetadataUpdates()

	return mp
}

// SetSource sets the audio source for this mountpoint
// Deprecated: Use TrySetSource for race-free source assignment
func (m *Mountpoint) SetSource(source io.ReadCloser, contentType string) error {
	m.sourceMu.Lock()
	defer m.sourceMu.Unlock()

	// Close existing source if present
	if m.source != nil {
		m.source.Close()
	}

	m.source = source
	m.sourceActive = true
	m.contentType = contentType

	m.logger.Printf("Source connected to mountpoint: %s (type: %s)", m.path, contentType)

	// Start reading from source
	go m.readFromSource()

	return nil
}

// TrySetSource atomically checks if source is available and sets it if free
// SECURITY: Prevents race condition where two sources try to connect simultaneously
// Returns (true, nil) if source was successfully set
// Returns (false, error) if mountpoint already has an active source
func (m *Mountpoint) TrySetSource(source io.ReadCloser, contentType string) (bool, error) {
	m.sourceMu.Lock()
	defer m.sourceMu.Unlock()

	// CRITICAL: Check active status INSIDE the lock to prevent race condition
	if m.sourceActive {
		return false, fmt.Errorf("mountpoint already has active source")
	}

	// Close existing source if present (should not happen, but defensive programming)
	if m.source != nil {
		m.source.Close()
	}

	m.source = source
	m.sourceActive = true
	m.contentType = contentType

	m.logger.Printf("Source connected to mountpoint: %s (type: %s)", m.path, contentType)

	// Start reading from source
	go m.readFromSource()

	return true, nil
}

// RemoveSource removes the current source
func (m *Mountpoint) RemoveSource() {
	m.sourceMu.Lock()
	defer m.sourceMu.Unlock()

	if m.source != nil {
		m.source.Close()
		m.source = nil
	}
	m.sourceActive = false

	m.logger.Printf("Source disconnected from mountpoint: %s", m.path)
}

// IsOnline returns whether the mountpoint has an active source
func (m *Mountpoint) IsOnline() bool {
	m.sourceMu.RLock()
	defer m.sourceMu.RUnlock()
	return m.sourceActive
}

// readFromSource reads data from the source and distributes to listeners
func (m *Mountpoint) readFromSource() {
	defer func() {
		m.sourceMu.Lock()
		m.sourceActive = false
		m.sourceMu.Unlock()
		m.logger.Printf("Source reader stopped for mountpoint: %s", m.path)
	}()

	// Use 4KB chunks for better efficiency
	// Balanced size: not too large to cause blocking, not too small to create overhead
	buffer := make([]byte, 4096)

	for {
		// Check if source is still active
		m.sourceMu.RLock()
		source := m.source
		active := m.sourceActive
		m.sourceMu.RUnlock()

		if !active || source == nil {
			return
		}

		// Read from source with timeout
		n, err := source.Read(buffer)
		if err != nil {
			if err != io.EOF {
				m.logger.Printf("Error reading from source on %s: %v", m.path, err)
			}
			return
		}

		if n > 0 {
			// Distribute data to all listeners
			chunk := make([]byte, n)
			copy(chunk, buffer[:n])
			m.distributeToListeners(chunk)
		}
	}
}

// distributeToListeners sends data to all connected listeners and updates ring buffer
func (m *Mountpoint) distributeToListeners(data []byte) {
	// Add to ring buffer for burst-on-connect
	m.ringBufferMu.Lock()
	m.ringBuffer[m.ringBufferPos] = data
	m.ringBufferPos = (m.ringBufferPos + 1) % m.ringBufferSize
	m.ringBufferMu.Unlock()

	// Distribute to all listeners
	m.listenersMu.RLock()
	defer m.listenersMu.RUnlock()

	for id, listener := range m.listeners {
		select {
		case listener.Chan <- data:
			listener.BytesSent += int64(len(data))
		default:
			// Listener's buffer is full (slow client or network)
			// With optimized burst (48 chunks) and larger buffer (1500 chunks),
			// this should rarely happen
			m.logger.Printf("Slow listener on %s (ID: %s), buffer full - skipping chunk", m.path, id)
		}
	}
}

// sendBurst sends buffered data to a new listener (burst-on-connect)
func (m *Mountpoint) sendBurst(listener *Listener) {
	// For fMP4 streams, send init segment first (critical!)
	m.initSegmentMu.RLock()
	initSeg := m.initSegment
	m.initSegmentMu.RUnlock()

	if initSeg != nil && len(initSeg) > 0 {
		select {
		case listener.Chan <- initSeg:
			listener.BytesSent += int64(len(initSeg))
			m.logger.Printf("Sent init segment to new listener (%d bytes)", len(initSeg))
		default:
			// Channel full, listener can't receive init segment - abort
			m.logger.Printf("WARNING: Could not send init segment to listener, channel full")
			return
		}
	}

	m.ringBufferMu.RLock()
	defer m.ringBufferMu.RUnlock()

	// Send buffered chunks gradually to avoid overwhelming the listener
	// Optimized burst size to prevent buffer overflow
	startPos := m.ringBufferPos
	sent := 0
	targetBurst := 48 // Send ~48 chunks = ~192KB (enough for instant start without overflow)

	for i := 0; i < m.ringBufferSize && sent < targetBurst; i++ {
		pos := (startPos + i) % m.ringBufferSize
		chunk := m.ringBuffer[pos]

		if chunk != nil && len(chunk) > 0 {
			select {
			case listener.Chan <- chunk:
				listener.BytesSent += int64(len(chunk))
				sent++

				// Add small delay every 16 chunks to allow browser to start consuming
				// This prevents instant buffer fill and gives HTTP handler time to flush
				if sent%16 == 0 {
					time.Sleep(5 * time.Millisecond)
				}
			default:
				// Channel full, stop burst
				return
			}
		}
	}

	// Burst completed
}

// AddListener adds a new listener to this mountpoint (simplified for browser-only)
func (m *Mountpoint) AddListener() *Listener {
	m.listenersMu.Lock()
	defer m.listenersMu.Unlock()

	m.nextID++
	id := m.path + ":" + time.Now().Format("20060102150405") + ":" + string(rune(m.nextID))

	listener := &Listener{
		ID:          id,
		Chan:        make(chan []byte, 1500), // Large buffer for browsers on Yggdrasil (1500*4KB = 6MB)
		ConnectedAt: time.Now(),
		BytesSent:   0,
	}

	m.listeners[id] = listener

	// Always send burst-on-connect for browsers to fill prebuffer quickly
	go m.sendBurst(listener)

	// Listener added (log removed to reduce noise)

	return listener
}

// RemoveListener removes a listener from this mountpoint
func (m *Mountpoint) RemoveListener(listener *Listener) {
	m.listenersMu.Lock()

	// SECURITY FIX: Remove from map BEFORE closing channel to prevent race condition
	// This ensures distributeToListeners cannot send to this channel after we close it
	if _, exists := m.listeners[listener.ID]; exists {
		delete(m.listeners, listener.ID)
		m.listenersMu.Unlock()

		// Now safe to close the channel - it's no longer accessible from the map
		close(listener.Chan)
		m.logger.Printf("Listener removed from %s (ID: %s, duration: %v, bytes: %d)",
			m.path, listener.ID, time.Since(listener.ConnectedAt), listener.BytesSent)
		return
	}

	m.listenersMu.Unlock()
}

// GetListenerCount returns the current number of listeners
func (m *Mountpoint) GetListenerCount() int {
	m.listenersMu.RLock()
	defer m.listenersMu.RUnlock()
	return len(m.listeners)
}

// UpdateMetadata updates the current playing metadata
func (m *Mountpoint) UpdateMetadata(metadata string) {
	select {
	case m.metadataChan <- metadata:
	default:
		m.logger.Printf("Warning: metadata channel full for %s", m.path)
	}
}

// GetMetadata returns the current metadata
func (m *Mountpoint) GetMetadata() string {
	m.metadataMu.RLock()
	defer m.metadataMu.RUnlock()
	return m.currentMetadata
}

// processMetadataUpdates processes metadata updates in the background
func (m *Mountpoint) processMetadataUpdates() {
	for {
		select {
		case metadata := <-m.metadataChan:
			m.metadataMu.Lock()
			m.currentMetadata = metadata
			m.metadataMu.Unlock()
			m.logger.Printf("Metadata updated for %s: %s", m.path, metadata)
		case <-m.stopChan:
			return
		}
	}
}

// Close closes the mountpoint and disconnects all listeners
func (m *Mountpoint) Close() {
	m.logger.Printf("Closing mountpoint: %s", m.path)

	// Stop metadata processor
	close(m.stopChan)

	// Remove source
	m.RemoveSource()

	// Disconnect all listeners
	m.listenersMu.Lock()
	for _, listener := range m.listeners {
		close(listener.Chan)
	}
	m.listeners = make(map[string]*Listener)
	m.listenersMu.Unlock()
}

// GetPath returns the mountpoint path
func (m *Mountpoint) GetPath() string {
	return m.path
}

// GetStationID returns the station ID
func (m *Mountpoint) GetStationID() int64 {
	return m.stationID
}

// ContentType returns the content type
func (m *Mountpoint) ContentType() string {
	m.sourceMu.RLock()
	defer m.sourceMu.RUnlock()
	return m.contentType
}

// Name returns the station name
func (m *Mountpoint) Name() string {
	return m.name
}

// Genre returns the station genre
func (m *Mountpoint) Genre() string {
	return m.genre
}

// Bitrate returns the stream bitrate
func (m *Mountpoint) Bitrate() int {
	return m.bitrate
}

// GetStats returns statistics about the mountpoint
func (m *Mountpoint) GetStats() map[string]interface{} {
	m.listenersMu.RLock()
	listenerCount := len(m.listeners)
	m.listenersMu.RUnlock()

	m.sourceMu.RLock()
	online := m.sourceActive
	m.sourceMu.RUnlock()

	m.metadataMu.RLock()
	metadata := m.currentMetadata
	m.metadataMu.RUnlock()

	return map[string]interface{}{
		"path":           m.path,
		"station_id":     m.stationID,
		"online":         online,
		"listeners":      listenerCount,
		"content_type":   m.contentType,
		"bitrate":        m.bitrate,
		"current_track":  metadata,
	}
}

// SetContentType updates the content type of the mountpoint
func (m *Mountpoint) SetContentType(contentType string) {
	m.sourceMu.Lock()
	defer m.sourceMu.Unlock()
	m.contentType = contentType
}

// SetInitSegment stores the initialization segment for fMP4 streams
// This segment will be sent to every new listener before media segments
func (m *Mountpoint) SetInitSegment(data []byte) {
	m.initSegmentMu.Lock()
	defer m.initSegmentMu.Unlock()
	// Make a copy to avoid external modifications
	m.initSegment = make([]byte, len(data))
	copy(m.initSegment, data)
	m.logger.Printf("Init segment stored for %s (%d bytes)", m.path, len(data))
}

// MountpointManager manages all active mountpoints
type MountpointManager struct {
	mountpoints sync.Map // map[string]*Mountpoint
	logger      *log.Logger
}

// NewMountpointManager creates a new mountpoint manager
func NewMountpointManager(logger *log.Logger) *MountpointManager {
	return &MountpointManager{
		logger: logger,
	}
}

// GetOrCreate gets an existing mountpoint or creates a new one
func (mm *MountpointManager) GetOrCreate(path string, stationID int64, name, genre, contentType string, bitrate int) *Mountpoint {
	if mp, exists := mm.mountpoints.Load(path); exists {
		return mp.(*Mountpoint)
	}

	mp := NewMountpoint(path, stationID, name, genre, contentType, bitrate, mm.logger)
	mm.mountpoints.Store(path, mp)

	mm.logger.Printf("Mountpoint created: %s", path)
	return mp
}

// Get retrieves a mountpoint by path
func (mm *MountpointManager) Get(path string) (*Mountpoint, bool) {
	mp, exists := mm.mountpoints.Load(path)
	if !exists {
		return nil, false
	}
	return mp.(*Mountpoint), true
}

// Remove removes a mountpoint
func (mm *MountpointManager) Remove(path string) {
	if mp, exists := mm.mountpoints.Load(path); exists {
		mountpoint := mp.(*Mountpoint)
		mountpoint.Close()
		mm.mountpoints.Delete(path)
		mm.logger.Printf("Mountpoint removed: %s", path)
	}
}

// List returns all active mountpoints
func (mm *MountpointManager) List() []*Mountpoint {
	var mountpoints []*Mountpoint

	mm.mountpoints.Range(func(key, value interface{}) bool {
		mp := value.(*Mountpoint)
		mountpoints = append(mountpoints, mp)
		return true
	})

	return mountpoints
}

// GetTotalListeners returns the total number of listeners across all mountpoints
func (mm *MountpointManager) GetTotalListeners() int {
	total := 0

	mm.mountpoints.Range(func(key, value interface{}) bool {
		mp := value.(*Mountpoint)
		total += mp.GetListenerCount()
		return true
	})

	return total
}

// CloseAll closes all mountpoints
func (mm *MountpointManager) CloseAll() {
	mm.logger.Println("Closing all mountpoints...")

	mm.mountpoints.Range(func(key, value interface{}) bool {
		mp := value.(*Mountpoint)
		mp.Close()
		return true
	})

	mm.mountpoints = sync.Map{}
}
