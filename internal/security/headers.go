package security

import (
	"fmt"
	"net/http"
	"strings"
)

// HeadersConfig configures security headers
type HeadersConfig struct {
	// Content-Security-Policy
	CSPDefaultSrc       []string
	CSPScriptSrc        []string
	CSPStyleSrc         []string
	CSPImgSrc           []string
	CSPFontSrc          []string
	CSPConnectSrc       []string
	CSPMediaSrc         []string
	CSPFrameAncestors   []string
	CSPBaseURI          []string
	CSPFormAction       []string

	// HSTS
	HSTSMaxAge            int
	HSTSIncludeSubDomains bool
	HSTSPreload           bool

	// Referrer Policy
	ReferrerPolicy string

	// Permissions Policy
	PermissionsPolicy map[string][]string

	// Feature flags
	EnableXFrameOptions     bool
	EnableXContentTypeOptions bool
	EnableXXSSProtection    bool
}

// DefaultHeadersConfig returns secure default configuration
func DefaultHeadersConfig() *HeadersConfig {
	return &HeadersConfig{
		// CSP defaults
		CSPDefaultSrc:     []string{"'self'"},
		CSPScriptSrc:      []string{"'self'", "'unsafe-inline'"},
		CSPStyleSrc:       []string{"'self'", "'unsafe-inline'"},
		CSPImgSrc:         []string{"'self'", "data:", "https:"},
		CSPFontSrc:        []string{"'self'"},
		CSPConnectSrc:     []string{"'self'"},
		CSPMediaSrc:       []string{"'self'"},
		CSPFrameAncestors: []string{"'none'"},
		CSPBaseURI:        []string{"'self'"},
		CSPFormAction:     []string{"'self'"},

		// HSTS defaults
		HSTSMaxAge:            31536000, // 1 year
		HSTSIncludeSubDomains: true,
		HSTSPreload:           false,

		// Referrer Policy
		ReferrerPolicy: "strict-origin-when-cross-origin",

		// Permissions Policy
		PermissionsPolicy: map[string][]string{
			"geolocation": {},
			"microphone":  {},
			"camera":      {},
		},

		// Feature flags
		EnableXFrameOptions:     true,
		EnableXContentTypeOptions: true,
		EnableXXSSProtection:    true,
	}
}

// HeadersManager manages security headers
type HeadersManager struct {
	config *HeadersConfig
	csp    string
	hsts   string
	pp     string
}

// NewHeadersManager creates a new headers manager
func NewHeadersManager(cfg *HeadersConfig) *HeadersManager {
	if cfg == nil {
		cfg = DefaultHeadersConfig()
	}

	hm := &HeadersManager{
		config: cfg,
	}

	// Pre-build CSP header
	hm.buildCSP()
	// Pre-build HSTS header
	hm.buildHSTS()
	// Pre-build Permissions Policy header
	hm.buildPermissionsPolicy()

	return hm
}

// buildCSP builds the Content-Security-Policy header value
func (hm *HeadersManager) buildCSP() {
	var directives []string

	if len(hm.config.CSPDefaultSrc) > 0 {
		directives = append(directives, "default-src "+strings.Join(hm.config.CSPDefaultSrc, " "))
	}
	if len(hm.config.CSPScriptSrc) > 0 {
		directives = append(directives, "script-src "+strings.Join(hm.config.CSPScriptSrc, " "))
	}
	if len(hm.config.CSPStyleSrc) > 0 {
		directives = append(directives, "style-src "+strings.Join(hm.config.CSPStyleSrc, " "))
	}
	if len(hm.config.CSPImgSrc) > 0 {
		directives = append(directives, "img-src "+strings.Join(hm.config.CSPImgSrc, " "))
	}
	if len(hm.config.CSPFontSrc) > 0 {
		directives = append(directives, "font-src "+strings.Join(hm.config.CSPFontSrc, " "))
	}
	if len(hm.config.CSPConnectSrc) > 0 {
		directives = append(directives, "connect-src "+strings.Join(hm.config.CSPConnectSrc, " "))
	}
	if len(hm.config.CSPMediaSrc) > 0 {
		directives = append(directives, "media-src "+strings.Join(hm.config.CSPMediaSrc, " "))
	}
	if len(hm.config.CSPFrameAncestors) > 0 {
		directives = append(directives, "frame-ancestors "+strings.Join(hm.config.CSPFrameAncestors, " "))
	}
	if len(hm.config.CSPBaseURI) > 0 {
		directives = append(directives, "base-uri "+strings.Join(hm.config.CSPBaseURI, " "))
	}
	if len(hm.config.CSPFormAction) > 0 {
		directives = append(directives, "form-action "+strings.Join(hm.config.CSPFormAction, " "))
	}

	hm.csp = strings.Join(directives, "; ")
}

// buildHSTS builds the Strict-Transport-Security header value
func (hm *HeadersManager) buildHSTS() {
	hsts := fmt.Sprintf("max-age=%d", hm.config.HSTSMaxAge)

	if hm.config.HSTSIncludeSubDomains {
		hsts += "; includeSubDomains"
	}

	if hm.config.HSTSPreload {
		hsts += "; preload"
	}

	hm.hsts = hsts
}

// buildPermissionsPolicy builds the Permissions-Policy header value
func (hm *HeadersManager) buildPermissionsPolicy() {
	var policies []string

	for feature, allowlist := range hm.config.PermissionsPolicy {
		if len(allowlist) == 0 {
			policies = append(policies, fmt.Sprintf("%s=()", feature))
		} else {
			allowStr := strings.Join(allowlist, " ")
			policies = append(policies, fmt.Sprintf("%s=(%s)", feature, allowStr))
		}
	}

	hm.pp = strings.Join(policies, ", ")
}

// Apply applies security headers to a response
func (hm *HeadersManager) Apply(w http.ResponseWriter) {
	// X-Frame-Options
	if hm.config.EnableXFrameOptions {
		w.Header().Set("X-Frame-Options", "DENY")
	}

	// X-Content-Type-Options
	if hm.config.EnableXContentTypeOptions {
		w.Header().Set("X-Content-Type-Options", "nosniff")
	}

	// X-XSS-Protection
	if hm.config.EnableXXSSProtection {
		w.Header().Set("X-XSS-Protection", "1; mode=block")
	}

	// Content-Security-Policy
	if hm.csp != "" {
		w.Header().Set("Content-Security-Policy", hm.csp)
	}

	// Strict-Transport-Security
	if hm.hsts != "" {
		w.Header().Set("Strict-Transport-Security", hm.hsts)
	}

	// Referrer-Policy
	if hm.config.ReferrerPolicy != "" {
		w.Header().Set("Referrer-Policy", hm.config.ReferrerPolicy)
	}

	// Permissions-Policy
	if hm.pp != "" {
		w.Header().Set("Permissions-Policy", hm.pp)
	}
}

// Middleware returns an HTTP middleware that applies security headers
func (hm *HeadersManager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hm.Apply(w)
		next.ServeHTTP(w, r)
	})
}

// SetCSPNonce sets a nonce for inline scripts/styles
func (hm *HeadersManager) SetCSPNonce(w http.ResponseWriter, nonce string) {
	// Add nonce to existing CSP
	csp := hm.csp

	// Replace 'unsafe-inline' with nonce for scripts
	csp = strings.ReplaceAll(csp, "script-src 'self' 'unsafe-inline'",
		fmt.Sprintf("script-src 'self' 'nonce-%s'", nonce))

	// Replace 'unsafe-inline' with nonce for styles
	csp = strings.ReplaceAll(csp, "style-src 'self' 'unsafe-inline'",
		fmt.Sprintf("style-src 'self' 'nonce-%s'", nonce))

	w.Header().Set("Content-Security-Policy", csp)
}

// DisableCache sets headers to prevent caching
func DisableCache(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate, private")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

// EnableCaching sets headers to allow caching
func EnableCaching(w http.ResponseWriter, maxAge int) {
	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", maxAge))
}

// SetContentType sets the Content-Type header safely
func SetContentType(w http.ResponseWriter, contentType string) {
	w.Header().Set("Content-Type", contentType)
	// Also set X-Content-Type-Options to prevent MIME sniffing
	w.Header().Set("X-Content-Type-Options", "nosniff")
}
