package crawler

import (
	"testing"
	"time"
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
