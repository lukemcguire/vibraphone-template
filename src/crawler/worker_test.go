package crawler

import (
	"testing"
	"time"
)

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
