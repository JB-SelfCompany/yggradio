package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGzip_CompressesAPIResponses verifies that API responses are compressed
func TestGzip_CompressesAPIResponses(t *testing.T) {
	// Create a large JSON response (should compress well)
	largeJSON := `{"data":["` + strings.Repeat("test", 1000) + `"]}`

	handler := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(largeJSON))
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Check headers
	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("Content-Encoding header not set, got: %s", w.Header().Get("Content-Encoding"))
	}

	if w.Header().Get("Vary") != "Accept-Encoding" {
		t.Errorf("Vary header not set correctly, got: %s", w.Header().Get("Vary"))
	}

	// Decompress response
	gr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer gr.Close()

	decompressed, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("Failed to decompress response: %v", err)
	}

	// Verify decompressed content matches original
	if string(decompressed) != largeJSON {
		t.Errorf("Decompressed content doesn't match original")
	}

	// Check compression ratio (should be at least 60% for repetitive JSON)
	originalSize := len(largeJSON)
	compressedSize := w.Body.Len()
	compressionRatio := float64(compressedSize) / float64(originalSize)

	if compressionRatio > 0.4 { // Less than 60% compression
		t.Errorf("Poor compression ratio: %.2f%% (expected < 40%%, got compressed=%d original=%d)",
			compressionRatio*100, compressedSize, originalSize)
	}

	t.Logf("Compression ratio: %.2f%% (original=%d, compressed=%d)",
		compressionRatio*100, originalSize, compressedSize)
}

// TestGzip_SkipsStreamingEndpoints verifies that streaming endpoints are not compressed
func TestGzip_SkipsStreamingEndpoints(t *testing.T) {
	testCases := []struct {
		name string
		path string
	}{
		{"Mountpoint stream", "/my-station"},
		{"Nested mountpoint", "/radio/station1"},
		{"Metadata update", "/my-station?mode=updinfo&song=Artist+-+Title"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "audio/mpeg")
				w.Write([]byte("fake audio data"))
			}))

			req := httptest.NewRequest("GET", tc.path, nil)
			req.Header.Set("Accept-Encoding", "gzip")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			// Should NOT have compression headers
			if w.Header().Get("Content-Encoding") == "gzip" {
				t.Errorf("Streaming endpoint %s should not be compressed", tc.path)
			}

			// Response should be uncompressed
			if w.Body.String() != "fake audio data" {
				t.Errorf("Streaming response was compressed when it shouldn't be")
			}
		})
	}
}

// TestGzip_SkipsPUTRequests verifies that PUT requests (streaming sources) are not compressed
func TestGzip_SkipsPUTRequests(t *testing.T) {
	handler := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	req := httptest.NewRequest("PUT", "/my-station", bytes.NewBufferString("audio stream data"))
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should NOT have compression headers
	if w.Header().Get("Content-Encoding") == "gzip" {
		t.Error("PUT request should not be compressed")
	}

	// Response should be uncompressed
	if w.Body.String() != "OK" {
		t.Error("PUT response was compressed when it shouldn't be")
	}
}

// TestGzip_SkipsWithoutAcceptEncoding verifies that compression is skipped without Accept-Encoding header
func TestGzip_SkipsWithoutAcceptEncoding(t *testing.T) {
	testData := "This is test data that would normally be compressed"

	handler := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(testData))
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	// Note: No Accept-Encoding header
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should NOT have compression headers
	if w.Header().Get("Content-Encoding") == "gzip" {
		t.Error("Response should not be compressed without Accept-Encoding header")
	}

	// Response should be uncompressed
	if w.Body.String() != testData {
		t.Errorf("Response doesn't match original. Got: %s, Want: %s", w.Body.String(), testData)
	}
}

// TestGzip_SkipsNonGzipAcceptEncoding verifies compression is skipped for other encodings
func TestGzip_SkipsNonGzipAcceptEncoding(t *testing.T) {
	testData := "Test data"

	handler := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(testData))
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Accept-Encoding", "deflate, br") // No gzip
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should NOT have compression headers
	if w.Header().Get("Content-Encoding") == "gzip" {
		t.Error("Response should not be compressed without gzip in Accept-Encoding")
	}

	if w.Body.String() != testData {
		t.Error("Response was modified when it shouldn't be")
	}
}

// TestGzip_AllowsStaticAssets verifies that static assets are compressed
func TestGzip_AllowsStaticAssets(t *testing.T) {
	testCases := []struct {
		name string
		path string
	}{
		{"JavaScript file", "/assets/main.js"},
		{"CSS file", "/assets/style.css"},
		{"Root HTML", "/"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			largeContent := strings.Repeat("test content ", 1000)

			handler := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(largeContent))
			}))

			req := httptest.NewRequest("GET", tc.path, nil)
			req.Header.Set("Accept-Encoding", "gzip")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			// Should have compression headers
			if w.Header().Get("Content-Encoding") != "gzip" {
				t.Errorf("Static asset %s should be compressed", tc.path)
			}

			// Verify content can be decompressed
			gr, err := gzip.NewReader(w.Body)
			if err != nil {
				t.Fatalf("Failed to create gzip reader: %v", err)
			}
			defer gr.Close()

			decompressed, err := io.ReadAll(gr)
			if err != nil {
				t.Fatalf("Failed to decompress: %v", err)
			}

			if string(decompressed) != largeContent {
				t.Error("Decompressed content doesn't match original")
			}
		})
	}
}

// TestGzip_HandlesEmptyResponses verifies that empty responses are handled correctly
func TestGzip_HandlesEmptyResponses(t *testing.T) {
	handler := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		// No body
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Status code changed, got: %d, want: %d", w.Code, http.StatusNoContent)
	}

	// Empty response should still work
	if w.Body.Len() > 0 {
		// Small amount of data is OK (gzip header), but shouldn't be large
		if w.Body.Len() > 50 {
			t.Errorf("Empty response has unexpected body size: %d bytes", w.Body.Len())
		}
	}
}

// TestGzip_MultipleAcceptEncodings verifies gzip works with multiple accepted encodings
func TestGzip_MultipleAcceptEncodings(t *testing.T) {
	testData := strings.Repeat("compress this ", 100)

	handler := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(testData))
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Accept-Encoding", "deflate, gzip, br") // gzip is in the list
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should use gzip since it's supported
	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("Should use gzip encoding, got: %s", w.Header().Get("Content-Encoding"))
	}

	// Verify decompression
	gr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer gr.Close()

	decompressed, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("Failed to decompress: %v", err)
	}

	if string(decompressed) != testData {
		t.Error("Decompressed content doesn't match original")
	}
}
