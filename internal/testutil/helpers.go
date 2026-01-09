package testutil

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// GenerateTestKeyPair generates an Ed25519 key pair for testing
func GenerateTestKeyPair(t *testing.T) (publicKey, privateKey string) {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	return hex.EncodeToString(pub), hex.EncodeToString(priv)
}

// SignMessage signs a message with Ed25519 private key
func SignMessage(t *testing.T, message string, privateKeyHex string) string {
	t.Helper()

	privKey, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		t.Fatalf("failed to decode private key: %v", err)
	}

	signature := ed25519.Sign(privKey, []byte(message))
	return hex.EncodeToString(signature)
}

// CreateTestRequest creates an HTTP test request with optional authentication
func CreateTestRequest(t *testing.T, method, path string, body []byte, authenticated bool, pubkey, signature, timestamp string) *http.Request {
	t.Helper()

	var req *http.Request
	var err error

	if body != nil {
		req = httptest.NewRequest(method, path, nil)
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}

	if authenticated {
		req.Header.Set("X-Pubkey", pubkey)
		req.Header.Set("X-Signature", signature)
		req.Header.Set("X-Timestamp", timestamp)
	}

	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	return req
}

// GetCurrentTimestamp returns current Unix timestamp as string
func GetCurrentTimestamp() string {
	return time.Now().Format("1234567890")
}

// AssertStatusCode asserts HTTP status code
func AssertStatusCode(t *testing.T, w *httptest.ResponseRecorder, expected int) {
	t.Helper()

	if w.Code != expected {
		t.Errorf("expected status code %d, got %d", expected, w.Code)
	}
}

// AssertContains asserts that response body contains substring
func AssertContains(t *testing.T, body, substring string) {
	t.Helper()

	if !contains(body, substring) {
		t.Errorf("expected body to contain %q, got %q", substring, body)
	}
}

// AssertNotContains asserts that response body does not contain substring
func AssertNotContains(t *testing.T, body, substring string) {
	t.Helper()

	if contains(body, substring) {
		t.Errorf("expected body not to contain %q, got %q", substring, body)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || indexSubstring(s, substr) >= 0)
}

func indexSubstring(s, substr string) int {
	n := len(substr)
	if n == 0 {
		return 0
	}
	for i := 0; i <= len(s)-n; i++ {
		if s[i:i+n] == substr {
			return i
		}
	}
	return -1
}

// MockTime is a helper for testing time-dependent code
type MockTime struct {
	current time.Time
}

// NewMockTime creates a new mock time
func NewMockTime(t time.Time) *MockTime {
	return &MockTime{current: t}
}

// Now returns the current mock time
func (m *MockTime) Now() time.Time {
	return m.current
}

// Advance advances the mock time by duration
func (m *MockTime) Advance(d time.Duration) {
	m.current = m.current.Add(d)
}

// TestHTTPHandler is a helper for testing HTTP handlers
func TestHTTPHandler(t *testing.T, handler http.HandlerFunc, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, nil)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	w := httptest.NewRecorder()
	handler(w, req)

	return w
}

// AssertNoError asserts that error is nil
func AssertNoError(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// AssertError asserts that error is not nil
func AssertError(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// AssertEqual asserts that two values are equal
func AssertEqual(t *testing.T, expected, actual interface{}) {
	t.Helper()

	if expected != actual {
		t.Errorf("expected %v, got %v", expected, actual)
	}
}

// AssertNotEqual asserts that two values are not equal
func AssertNotEqual(t *testing.T, expected, actual interface{}) {
	t.Helper()

	if expected == actual {
		t.Errorf("expected values to be different, both are %v", expected)
	}
}

// AssertTrue asserts that condition is true
func AssertTrue(t *testing.T, condition bool, message string) {
	t.Helper()

	if !condition {
		t.Errorf("assertion failed: %s", message)
	}
}

// AssertFalse asserts that condition is false
func AssertFalse(t *testing.T, condition bool, message string) {
	t.Helper()

	if condition {
		t.Errorf("assertion failed: %s", message)
	}
}
