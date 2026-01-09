package federation_server

import (
	"database/sql"
	"time"
)

// Node represents a registered YggRadio node
type Node struct {
	ID                  int64
	UUID                string
	Address             string
	Port                int
	Pubkey              string
	Name                string
	Description         sql.NullString
	Version             sql.NullString
	FirstRegistered     time.Time
	LastSeen            time.Time
	LastPullAttempt     sql.NullTime
	LastPullSuccess     sql.NullTime
	Status              string // pending, active, offline, blocked
	ConsecutiveFailures int
	TotalPulls          int
	TotalStations       int
}

// Station represents a federated radio station from a node
type Station struct {
	ID             int64
	UUID           string
	NodeUUID       string
	Name           string
	Description    sql.NullString
	Mountpoint     string
	OwnerPubkey    string
	Status         string // online, offline
	ListenersCount int
	ContentType    sql.NullString
	Bitrate        sql.NullInt64
	Genre          sql.NullString
	MetadataTitle  sql.NullString
	AverageRating  float64
	VoteCount      int
	CreatedAt      sql.NullTime
	UpdatedAt      sql.NullTime
	FirstSeen      time.Time
	LastSeen       time.Time
}

// PullHistory represents an audit record of a pull operation
type PullHistory struct {
	ID             int64
	NodeUUID       string
	PullTimestamp  time.Time
	Success        bool
	StationsPulled int
	ErrorMessage   sql.NullString
	DurationMs     sql.NullInt64
}

// SecurityAuditLog represents a security event
type SecurityAuditLog struct {
	ID          int64
	EventType   string
	Severity    string // low, medium, high, critical
	IPv6Address sql.NullString
	Pubkey      sql.NullString
	Endpoint    sql.NullString
	Details     sql.NullString
	CreatedAt   time.Time
}

// RegistrationRequest represents a node registration request
type RegistrationRequest struct {
	UUID        string `json:"uuid"`
	Address     string `json:"address"`
	Port        int    `json:"port"`
	Pubkey      string `json:"pubkey"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Signature   string `json:"signature"`
	Timestamp   int64  `json:"timestamp"`
}

// RegistrationResponse represents the response to a registration request
type RegistrationResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	NodeID  string `json:"node_id,omitempty"`
}

// StationListResponse represents the response for federated stations query
type StationListResponse struct {
	Success  bool              `json:"success"`
	Stations []StationPublic   `json:"stations"`
	Count    int               `json:"count"`
	Metadata ResponseMetadata  `json:"metadata"`
}

// StationPublic represents public station data for API responses
type StationPublic struct {
	UUID           string  `json:"uuid"`
	NodeUUID       string  `json:"node_uuid"`
	NodeAddress    string  `json:"node_address,omitempty"` // Yggdrasil IPv6 address
	NodeName       string  `json:"node_name,omitempty"`
	Name           string  `json:"name"`
	Description    string  `json:"description,omitempty"`
	Mountpoint     string  `json:"mountpoint"`
	OwnerPubkey    string  `json:"owner_pubkey"`
	Status         string  `json:"status"`
	ListenersCount int     `json:"listeners_count"`
	ContentType    string  `json:"content_type,omitempty"`
	Bitrate        int     `json:"bitrate,omitempty"`
	Genre          string  `json:"genre,omitempty"`
	MetadataTitle  string  `json:"metadata_title,omitempty"`
	AverageRating  float64 `json:"average_rating"`
	VoteCount      int     `json:"vote_count"`
	LastSeen       string  `json:"last_seen"`
}

// NodeListResponse represents the response for registered nodes query
type NodeListResponse struct {
	Success  bool             `json:"success"`
	Nodes    []NodePublic     `json:"nodes"`
	Count    int              `json:"count"`
	Metadata ResponseMetadata `json:"metadata"`
}

// NodePublic represents public node data for API responses
type NodePublic struct {
	UUID            string `json:"uuid"`
	Address         string `json:"address"`
	Port            int    `json:"port"`
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	Version         string `json:"version,omitempty"`
	Status          string `json:"status"`
	TotalStations   int    `json:"total_stations"`
	LastSeen        string `json:"last_seen"`
	FirstRegistered string `json:"first_registered"`
}

// ResponseMetadata contains pagination and timing info
type ResponseMetadata struct {
	Timestamp string `json:"timestamp"`
	Version   string `json:"version"`
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status  string            `json:"status"`
	Version string            `json:"version"`
	Uptime  string            `json:"uptime"`
	Stats   map[string]int    `json:"stats"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

// PulledStationData represents station data received from a node during pull
type PulledStationData struct {
	UUID           string  `json:"uuid"`
	Name           string  `json:"name"`
	Description    string  `json:"description"`
	Mountpoint     string  `json:"mountpoint"`
	OwnerPubkey    string  `json:"owner_pubkey"`
	Status         string  `json:"status"`
	ListenersCount int     `json:"listeners_count"`
	ContentType    string  `json:"content_type"`
	Bitrate        int     `json:"bitrate"`
	Genre          string  `json:"genre"`
	MetadataTitle  string  `json:"metadata_title"`
	AverageRating  float64 `json:"average_rating"`
	VoteCount      int     `json:"vote_count"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

// PullResponse represents the response from a node's station provider endpoint
type PullResponse struct {
	Success  bool                `json:"success"`
	Stations []PulledStationData `json:"stations"`
	Count    int                 `json:"count"`
	Pubkey   string              `json:"pubkey"`
	Signature string             `json:"signature"`
	Timestamp int64              `json:"timestamp"`
}
