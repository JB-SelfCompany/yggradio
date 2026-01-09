package web

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strings"
)

// Embed the built React frontend
//
//go:embed dist/*
var distFS embed.FS

// DistFS returns a filesystem containing the built frontend assets
func DistFS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}

// Handler serves the embedded frontend with proper fallback to index.html for client-side routing
type Handler struct {
	fileServer http.Handler
	logger     *log.Logger
}

// NewHandler creates a new web handler for serving the embedded frontend
func NewHandler(logger *log.Logger) (*Handler, error) {
	dist, err := DistFS()
	if err != nil {
		return nil, err
	}

	return &Handler{
		fileServer: http.FileServer(http.FS(dist)),
		logger:     logger,
	}, nil
}

// ServeHTTP implements http.Handler
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Security headers
	h.setSecurityHeaders(w)

	// Clean the path
	cleanPath := path.Clean(r.URL.Path)

	// Prevent directory traversal
	if strings.Contains(cleanPath, "..") {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	// Remove leading slash for embed.FS
	if strings.HasPrefix(cleanPath, "/") {
		cleanPath = cleanPath[1:]
	}

	// Check if file exists
	dist, err := DistFS()
	if err != nil {
		h.logger.Printf("Error accessing dist filesystem: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Try to open the requested file
	if _, err := fs.Stat(dist, cleanPath); err != nil {
		// File doesn't exist - serve index.html for client-side routing
		// This enables React Router to handle the route
		r.URL.Path = "/"
		h.fileServer.ServeHTTP(w, r)
		return
	}

	// File exists - determine content type and serve
	contentType := getContentType(cleanPath)
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}

	// Set caching headers for static assets
	if isStaticAsset(cleanPath) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache, must-revalidate")
	}

	h.fileServer.ServeHTTP(w, r)
}

// setSecurityHeaders sets security-related HTTP headers
func (h *Handler) setSecurityHeaders(w http.ResponseWriter) {
	// Prevent clickjacking
	w.Header().Set("X-Frame-Options", "DENY")

	// Prevent MIME sniffing
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// XSS protection
	w.Header().Set("X-XSS-Protection", "1; mode=block")

	// Referrer policy
	w.Header().Set("Referrer-Policy", "no-referrer")

	// Content Security Policy
	csp := []string{
		"default-src 'self'",
		"script-src 'self'",
		"style-src 'self' 'unsafe-inline'", // Tailwind requires inline styles
		"img-src 'self' data: https:",
		"font-src 'self'",
		"connect-src 'self'",
		"media-src 'self' blob:",
		"frame-ancestors 'none'",
		"base-uri 'self'",
		"form-action 'self'",
	}
	w.Header().Set("Content-Security-Policy", strings.Join(csp, "; "))

	// Permissions Policy (formerly Feature-Policy)
	w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
}

// getContentType returns the Content-Type for a given file path
func getContentType(filePath string) string {
	ext := path.Ext(filePath)
	switch ext {
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js", ".mjs":
		return "application/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".otf":
		return "font/otf"
	case ".ico":
		return "image/x-icon"
	default:
		return ""
	}
}

// isStaticAsset determines if a file should have long-term caching
func isStaticAsset(filePath string) bool {
	// Assets in the assets directory typically have hashed names
	// and can be cached indefinitely
	if strings.Contains(filePath, "/assets/") {
		return true
	}

	// Specific file types that are cacheable
	ext := path.Ext(filePath)
	switch ext {
	case ".js", ".mjs", ".css", ".woff", ".woff2", ".ttf", ".otf",
		".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".ico":
		return true
	default:
		return false
	}
}
