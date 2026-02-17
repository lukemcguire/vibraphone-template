package crawler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lukemcguire/zombiecrawl/result"
)

func TestIsBinaryContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		// PDF
		{"PDF", "application/pdf", true},
		{"PDF with charset", "application/pdf; charset=utf-8", true},

		// Images
		{"PNG", "image/png", true},
		{"JPEG", "image/jpeg", true},
		{"GIF", "image/gif", true},
		{"SVG", "image/svg+xml", true},
		{"WebP", "image/webp", true},
		{"ICO", "image/x-icon", true},
		{"BMP", "image/bmp", true},

		// Archives
		{"ZIP", "application/zip", true},
		{"ZIP compressed", "application/x-zip-compressed", true},
		{"GZIP", "application/gzip", true},
		{"RAR", "application/vnd.rar", true},
		{"7Z", "application/x-7z-compressed", true},

		// Binary/streams
		{"Octet stream", "application/octet-stream", true},

		// Video
		{"MP4", "video/mp4", true},
		{"WebM", "video/webm", true},
		{"MPEG", "video/mpeg", true},
		{"AVI", "video/x-msvideo", true},

		// Audio
		{"MP3", "audio/mpeg", true},
		{"WAV", "audio/wav", true},
		{"OGG", "audio/ogg", true},
		{"FLAC", "audio/flac", true},

		// Fonts
		{"WOFF", "font/woff", true},
		{"WOFF2", "font/woff2", true},
		{"TTF", "font/ttf", true},
		{"OTF", "font/otf", true},

		// Non-binary (HTML/text)
		{"HTML", "text/html", false},
		{"HTML with charset", "text/html; charset=utf-8", false},
		{"XHTML", "application/xhtml+xml", false},
		{"Plain text", "text/plain", false},
		{"CSS", "text/css", false},
		{"JavaScript", "application/javascript", false},
		{"JSON", "application/json", false},
		{"XML", "application/xml", false},
		{"Empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBinaryContentType(tt.contentType); got != tt.want {
				t.Errorf("isBinaryContentType(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig("https://example.com")

	if cfg.StartURL != "https://example.com" {
		t.Errorf("StartURL = %q, want %q", cfg.StartURL, "https://example.com")
	}
	if cfg.Concurrency != 17 {
		t.Errorf("Concurrency = %d, want 17", cfg.Concurrency)
	}
	if cfg.RequestTimeout != 10*time.Second {
		t.Errorf("RequestTimeout = %v, want 10s", cfg.RequestTimeout)
	}
}

// TestCheckURLMalformedHTML verifies that pages with malformed HTML that fails
// to parse are classified as broken links with CategoryMalformedHTML.
func TestCheckURLMalformedHTML(t *testing.T) {
	// Create a server that returns HTML with unparseable URLs
	// The html tokenizer is very tolerant, so we need to trigger errors
	// in the URL parsing/normalization phase
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		// Use a URL with a newline character which will fail normalization
		// (url.Parse accepts it but the URL is invalid)
		html := `<html><body><a href="http://example` + "\n" + `.com/page">Link</a></body></html>`
		if _, err := w.Write([]byte(html)); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer ts.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	cfg := DefaultConfig(ts.URL)
	job := CrawlJob{
		URL:        ts.URL,
		SourcePage: "",
		IsExternal: false,
	}

	res := CheckURL(context.Background(), client, job, cfg)

	// Should have a LinkResult indicating malformed HTML
	if res.Result == nil {
		t.Fatal("expected LinkResult for malformed HTML, got nil")
	}

	if res.Result.ErrorCategory != result.CategoryMalformedHTML {
		t.Errorf("ErrorCategory = %v, want %v", res.Result.ErrorCategory, result.CategoryMalformedHTML)
	}

	if !strings.Contains(res.Result.Error, "parse errors") {
		t.Errorf("Error = %q, want to contain 'parse errors'", res.Result.Error)
	}

	// Should have no extracted links
	if len(res.Links) != 0 {
		t.Errorf("Links = %v, want empty", res.Links)
	}
}

// TestCheckURLVerboseNetwork tests that verbose network diagnostics are
// included in error messages when VerboseNetwork is enabled.
func TestCheckURLVerboseNetwork(t *testing.T) {
	// Create a server that will timeout
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // Sleep longer than client timeout
	}))
	defer ts.Close()

	client := &http.Client{Timeout: 100 * time.Millisecond}
	cfg := DefaultConfig(ts.URL)
	cfg.VerboseNetwork = true // Enable verbose network diagnostics
	cfg.RequestTimeout = 100 * time.Millisecond

	job := CrawlJob{
		URL:        ts.URL,
		SourcePage: "",
		IsExternal: false,
	}

	res := CheckURL(context.Background(), client, job, cfg)

	if res.Result == nil {
		t.Fatal("expected LinkResult for timeout, got nil")
	}

	// With verbose enabled, the error should include timeout context
	// e.g., "Request timed out after 100ms"
	if !strings.Contains(strings.ToLower(res.Result.Error), "timed out") {
		t.Errorf("Error = %q, want to contain 'timed out'", res.Result.Error)
	}

	// Verbose errors should include duration information
	if !strings.Contains(res.Result.Error, "ms") && !strings.Contains(res.Result.Error, "s") {
		t.Errorf("Error = %q, want to contain duration info (ms or s)", res.Result.Error)
	}
}

// TestConfigVerboseNetworkField tests that the Config struct has a VerboseNetwork field.
func TestConfigVerboseNetworkField(t *testing.T) {
	cfg := Config{
		StartURL:        "https://example.com",
		VerboseNetwork:  true,
		Concurrency:     10,
		RequestTimeout:  10 * time.Second,
	}

	if !cfg.VerboseNetwork {
		t.Error("VerboseNetwork should be true")
	}

	// Test default is false
	defaultCfg := DefaultConfig("https://example.com")
	if defaultCfg.VerboseNetwork {
		t.Error("Default VerboseNetwork should be false")
	}
}
