package federation_server

import (
	"context"
	"log"
	"sync"
	"time"
)

// Scheduler manages periodic background tasks for the federation server
type Scheduler struct {
	db          *DB
	config      *Config
	logger      *log.Logger
	aggregator  *StationAggregator
	nodeManager *NodeManager

	// Cancellation and synchronization
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	running    bool
	runningMux sync.Mutex
}

// NewScheduler creates a new scheduler
func NewScheduler(
	db *DB,
	config *Config,
	logger *log.Logger,
	aggregator *StationAggregator,
	nodeManager *NodeManager,
) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	return &Scheduler{
		db:          db,
		config:      config,
		logger:      logger,
		aggregator:  aggregator,
		nodeManager: nodeManager,
		ctx:         ctx,
		cancel:      cancel,
		running:     false,
	}
}

// Start starts the background scheduler
func (s *Scheduler) Start() error {
	s.runningMux.Lock()
	defer s.runningMux.Unlock()

	if s.running {
		return nil // Already running
	}

	s.logger.Println("Starting federation scheduler...")

	// Start pull task
	s.wg.Add(1)
	go s.runPullTask()

	// Start maintenance task
	s.wg.Add(1)
	go s.runMaintenanceTask()

	s.running = true
	s.logger.Println("Federation scheduler started successfully")

	return nil
}

// Stop stops the background scheduler gracefully
func (s *Scheduler) Stop() error {
	s.runningMux.Lock()
	defer s.runningMux.Unlock()

	if !s.running {
		return nil // Already stopped
	}

	s.logger.Println("Stopping federation scheduler...")

	// Cancel context to signal all goroutines to stop
	s.cancel()

	// Wait for all tasks to complete (with timeout)
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Println("Federation scheduler stopped gracefully")
	case <-time.After(30 * time.Second):
		s.logger.Println("WARNING: Federation scheduler stop timeout - some tasks may still be running")
	}

	s.running = false
	return nil
}

// runPullTask periodically pulls station data from all registered nodes
func (s *Scheduler) runPullTask() {
	defer s.wg.Done()

	// Reduced logging: Only log at startup, not every cycle
	s.logger.Printf("Pull task initialized (interval: %ds)", s.config.Federation.PullInterval)

	// Run immediately on startup
	s.executePullCycle()

	// Create ticker for periodic pulls
	ticker := time.NewTicker(time.Duration(s.config.Federation.PullInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return

		case <-ticker.C:
			s.executePullCycle()
		}
	}
}

// executePullCycle executes a single pull cycle
func (s *Scheduler) executePullCycle() {
	startTime := time.Now()

	// Pull from all active nodes
	if err := s.aggregator.PullFromAllNodes(); err != nil {
		// Only log errors - successful pulls are silent to reduce log noise
		s.logger.Printf("ERROR: Pull cycle failed: %v", err)
	}

	// Log completion only if pull took significant time (>5 seconds) or in debug mode
	duration := time.Since(startTime)
	if duration > 5*time.Second {
		s.logger.Printf("Pull cycle completed in %v (longer than usual)", duration)
	}
}

// runMaintenanceTask periodically runs maintenance operations
func (s *Scheduler) runMaintenanceTask() {
	defer s.wg.Done()

	// Reduced logging: Only log at startup, not every cycle
	s.logger.Printf("Maintenance task initialized (interval: 5 minutes)")

	// Run immediately on startup
	s.executeMaintenanceCycle()

	// Create ticker for periodic maintenance (every 5 minutes)
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return

		case <-ticker.C:
			s.executeMaintenanceCycle()
		}
	}
}

// executeMaintenanceCycle executes a single maintenance cycle
func (s *Scheduler) executeMaintenanceCycle() {
	// Maintenance runs silently unless there's something to report
	// This reduces log noise for routine operations

	// 1. Mark stale nodes as offline
	if err := s.nodeManager.MarkStaleNodesOffline(); err != nil {
		s.logger.Printf("ERROR: Failed to mark stale nodes offline: %v", err)
	}

	// 2. Remove stale stations
	if err := s.aggregator.CleanupStaleStations(); err != nil {
		s.logger.Printf("ERROR: Failed to cleanup stale stations: %v", err)
	}

	// 3. Cleanup old pull history (keep last 7 days)
	if err := s.cleanupOldPullHistory(7); err != nil {
		s.logger.Printf("ERROR: Failed to cleanup old pull history: %v", err)
	}

	// 4. Cleanup old security audit logs (keep last 30 days)
	if err := s.cleanupOldSecurityLogs(30); err != nil {
		s.logger.Printf("ERROR: Failed to cleanup old security logs: %v", err)
	}

	// No completion message - successful maintenance is silent to reduce log noise
}

// cleanupOldPullHistory removes pull history older than N days
func (s *Scheduler) cleanupOldPullHistory(retentionDays int) error {
	query := `
		DELETE FROM pull_history
		WHERE pull_timestamp < datetime('now', '-' || ? || ' days')
	`
	result, err := s.db.Exec(query, retentionDays)
	if err != nil {
		return err
	}

	// Only log if we actually deleted something (reduces log noise)
	rows, _ := result.RowsAffected()
	if rows > 0 {
		s.logger.Printf("Maintenance: Cleaned up %d old pull history records (retention: %d days)", rows, retentionDays)
	}

	return nil
}

// cleanupOldSecurityLogs removes security logs older than N days
func (s *Scheduler) cleanupOldSecurityLogs(retentionDays int) error {
	query := `
		DELETE FROM security_audit_log
		WHERE created_at < datetime('now', '-' || ? || ' days')
	`
	result, err := s.db.Exec(query, retentionDays)
	if err != nil {
		return err
	}

	// Only log if we actually deleted something (reduces log noise)
	rows, _ := result.RowsAffected()
	if rows > 0 {
		s.logger.Printf("Maintenance: Cleaned up %d old security audit logs (retention: %d days)", rows, retentionDays)
	}

	return nil
}

// IsRunning returns whether the scheduler is currently running
func (s *Scheduler) IsRunning() bool {
	s.runningMux.Lock()
	defer s.runningMux.Unlock()
	return s.running
}

// TriggerPullNow manually triggers a pull cycle (for testing/admin use)
func (s *Scheduler) TriggerPullNow() {
	s.logger.Println("Manual pull trigger requested")
	go s.executePullCycle()
}

// TriggerMaintenanceNow manually triggers a maintenance cycle (for testing/admin use)
func (s *Scheduler) TriggerMaintenanceNow() {
	s.logger.Println("Manual maintenance trigger requested")
	go s.executeMaintenanceCycle()
}
