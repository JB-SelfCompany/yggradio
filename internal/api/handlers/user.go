package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	"github.com/JB-SelfCompany/yggradio/internal/database/models"
	"github.com/JB-SelfCompany/yggradio/internal/security"
)

// UserHandler handles user profile operations
type UserHandler struct {
	db          *sql.DB
	userRepo    *models.UserRepository
	validator   *security.Validator
	sanitizer   *security.Sanitizer
	auditLogger *security.AuditLogger
	logger      *log.Logger
}

// NewUserHandler creates a new user handler
func NewUserHandler(
	db *sql.DB,
	userRepo *models.UserRepository,
	validator *security.Validator,
	sanitizer *security.Sanitizer,
	auditLogger *security.AuditLogger,
	logger *log.Logger,
) *UserHandler {
	return &UserHandler{
		db:          db,
		userRepo:    userRepo,
		validator:   validator,
		sanitizer:   sanitizer,
		auditLogger: auditLogger,
		logger:      logger,
	}
}

// GetProfile returns the user's profile
func (h *UserHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get authentication method and user info from context
	authMethod, ok := r.Context().Value("auth_method").(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	userID, ok := r.Context().Value("user_id").(int64)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get user from database based on authentication method
	var user *models.User
	var err error

	if authMethod == "ed25519" {
		// Ed25519 authentication - get by pubkey
		pubkey, ok := r.Context().Value("pubkey").(string)
		if !ok || pubkey == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		user, err = h.userRepo.GetByPubkey(pubkey)
		if err != nil {
			h.logger.Printf("Database error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// If user doesn't exist, create them
		if user == nil {
			user, err = h.userRepo.Create(pubkey)
			if err != nil {
				h.logger.Printf("Failed to create user profile: %v", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		}
	} else {
		// Magic link authentication - get by user ID
		user, err = h.userRepo.GetByID(userID)
		if err != nil {
			h.logger.Printf("Database error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if user == nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
	}

	// Build response
	response := map[string]interface{}{
		"user_id":     user.ID,
		"created_at":  user.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"is_admin":    user.IsAdmin,
		"auth_method": authMethod,
	}

	// Add pubkey only for Ed25519 users
	if user.Pubkey.Valid {
		response["pubkey"] = user.Pubkey.String
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Note: UpdateProfile removed - users are now anonymous (privacy-first)
