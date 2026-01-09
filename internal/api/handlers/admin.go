package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/JB-SelfCompany/yggradio/internal/database/models"
	"github.com/JB-SelfCompany/yggradio/internal/moderation"
	"github.com/JB-SelfCompany/yggradio/internal/security"
)

// AdminHandler handles user management endpoints
// NOTE: All authenticated users can access these endpoints to manage their own data
type AdminHandler struct {
	db          *sql.DB
	modRepo     *models.ModerationRepository
	powManager  *moderation.ProofOfWork
	logger      *log.Logger
	validator   *security.Validator
	sanitizer   *security.Sanitizer
	csrfManager *security.CSRFManager
	auditLogger *security.AuditLogger
}

// NewAdminHandler creates a new admin handler
func NewAdminHandler(
	db *sql.DB,
	modRepo *models.ModerationRepository,
	powManager *moderation.ProofOfWork,
	logger *log.Logger,
	validator *security.Validator,
	sanitizer *security.Sanitizer,
	csrfManager *security.CSRFManager,
	auditLogger *security.AuditLogger,
) *AdminHandler {
	return &AdminHandler{
		db:          db,
		modRepo:     modRepo,
		powManager:  powManager,
		logger:      logger,
		validator:   validator,
		sanitizer:   sanitizer,
		csrfManager: csrfManager,
		auditLogger: auditLogger,
	}
}

// requireAdmin middleware checks if requester is authenticated
// NOTE: Any authenticated user can access admin endpoints for their own data
func (h *AdminHandler) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get pubkey from context (set by auth middleware)
		pubkey, ok := r.Context().Value("pubkey").(string)
		if !ok || pubkey == "" {
			h.logAudit("unauthorized_access", "high",
				"", r.URL.Path,
				"Management access attempted without authentication")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

// GetInstanceStats returns instance statistics
func (h *AdminHandler) GetInstanceStats(w http.ResponseWriter, r *http.Request) {
	stats := make(map[string]interface{})

	// Database stats
	dbStats := struct {
		Stations  int `json:"stations"`
		Users     int `json:"users"`
		Comments  int `json:"comments"`
		SizeBytes int `json:"size_bytes"`
	}{}

	// Count tables
	h.db.QueryRow("SELECT COUNT(*) FROM stations").Scan(&dbStats.Stations)
	h.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&dbStats.Users)
	h.db.QueryRow("SELECT COUNT(*) FROM comments").Scan(&dbStats.Comments)

	// Get database size
	var pageCount, pageSize int
	h.db.QueryRow("PRAGMA page_count").Scan(&pageCount)
	h.db.QueryRow("PRAGMA page_size").Scan(&pageSize)
	dbStats.SizeBytes = pageCount * pageSize

	stats["database"] = dbStats

	// Security audit stats
	var totalAuditEvents, criticalEvents, highEvents int
	h.db.QueryRow("SELECT COUNT(*) FROM security_audit_log").Scan(&totalAuditEvents)
	h.db.QueryRow("SELECT COUNT(*) FROM security_audit_log WHERE severity = 'critical'").Scan(&criticalEvents)
	h.db.QueryRow("SELECT COUNT(*) FROM security_audit_log WHERE severity = 'high'").Scan(&highEvents)

	stats["security"] = map[string]int{
		"total_audit_events": totalAuditEvents,
		"critical_events":    criticalEvents,
		"high_severity":      highEvents,
	}

	// PoW configuration
	stats["pow"] = map[string]int{
		"difficulty": h.powManager.GetDifficulty(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// GetAuditLog returns security audit log with filtering
func (h *AdminHandler) GetAuditLog(w http.ResponseWriter, r *http.Request) {
	// Parse parameters
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	severity := r.URL.Query().Get("severity")
	eventType := r.URL.Query().Get("event_type")

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	// Build query
	query := `
		SELECT id, event_type, severity, ipv6_address, pubkey, endpoint, details, created_at
		FROM security_audit_log
		WHERE 1=1
	`
	args := []interface{}{}

	if severity != "" {
		query += " AND severity = ?"
		args = append(args, severity)
	}

	if eventType != "" {
		query += " AND event_type = ?"
		args = append(args, eventType)
	}

	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	// Execute query
	rows, err := h.db.Query(query, args...)
	if err != nil {
		h.logger.Printf("ERROR: Failed to query audit log: %v", err)
		http.Error(w, "Failed to retrieve audit log", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// Parse results
	entries := []map[string]interface{}{}
	for rows.Next() {
		var (
			id          int64
			eventType   string
			severity    string
			ipv6        sql.NullString
			pubkey      sql.NullString
			endpoint    sql.NullString
			details     sql.NullString
			createdAt   time.Time
		)

		err := rows.Scan(&id, &eventType, &severity, &ipv6, &pubkey, &endpoint, &details, &createdAt)
		if err != nil {
			h.logger.Printf("ERROR: Failed to scan audit log row: %v", err)
			continue
		}

		entry := map[string]interface{}{
			"id":         id,
			"event_type": eventType,
			"severity":   severity,
			"created_at": createdAt,
		}

		if ipv6.Valid {
			entry["ipv6_address"] = ipv6.String
		}
		if pubkey.Valid {
			entry["pubkey"] = pubkey.String
		}
		if endpoint.Valid {
			entry["endpoint"] = endpoint.String
		}
		if details.Valid {
			entry["details"] = details.String
		}

		entries = append(entries, entry)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"entries": entries,
		"limit":   limit,
		"offset":  offset,
	})
}

// GetPoWConfig returns the current PoW configuration
func (h *AdminHandler) GetPoWConfig(w http.ResponseWriter, r *http.Request) {
	config := map[string]interface{}{
		"difficulty": h.powManager.GetDifficulty(),
		"estimate_time_4bit":  h.powManager.EstimateTime(4).String(),
		"estimate_time_8bit":  h.powManager.EstimateTime(8).String(),
		"estimate_time_12bit": h.powManager.EstimateTime(12).String(),
		"estimate_time_16bit": h.powManager.EstimateTime(16).String(),
		"estimate_time_20bit": h.powManager.EstimateTime(20).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// UpdatePoWConfig updates the PoW difficulty (requires restart to take effect)
func (h *AdminHandler) UpdatePoWConfig(w http.ResponseWriter, r *http.Request) {
	// Get admin pubkey from context
	adminPubkey, _ := r.Context().Value("pubkey").(string)

	var req struct {
		Difficulty int `json:"difficulty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate difficulty
	if req.Difficulty < 4 || req.Difficulty > 20 {
		http.Error(w, "Difficulty must be between 4 and 20", http.StatusBadRequest)
		return
	}

	// Note: This changes the in-memory configuration
	// For persistent changes, configuration file needs to be updated
	h.powManager = moderation.NewProofOfWork(req.Difficulty)

	// Log action
	h.logAudit("config_change", "high",
		adminPubkey, r.URL.Path,
		"Changed PoW difficulty to "+strconv.Itoa(req.Difficulty))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "success",
		"message":    "PoW difficulty updated (in-memory only, restart required for persistence)",
		"difficulty": req.Difficulty,
	})
}

// logAudit logs a security audit event
func (h *AdminHandler) logAudit(eventType, severity, pubkey, endpoint, details string) {
	if h.auditLogger != nil {
		h.auditLogger.Log(eventType, severity, pubkey, endpoint, details)
	} else {
		// Fallback to database logging
		query := `
			INSERT INTO security_audit_log (event_type, severity, pubkey, endpoint, details)
			VALUES (?, ?, ?, ?, ?)
		`
		h.db.Exec(query, eventType, severity, pubkey, endpoint, details)
	}
}
