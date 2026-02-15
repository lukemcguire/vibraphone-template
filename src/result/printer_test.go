package result

import (
	"bytes"
	"testing"
	"time"
)

func TestPrintResults_NoBrokenLinks(t *testing.T) {
	var buf bytes.Buffer
	r := &Result{
		Stats: CrawlStats{TotalChecked: 10, BrokenCount: 0, Duration: time.Second},
	}

	PrintResults(&buf, r)

	got := buf.String()
	want := "No broken links found!\nChecked 10 URLs, found 0 broken links\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrintResults_WithBrokenLinks(t *testing.T) {
	var buf bytes.Buffer
	r := &Result{
		BrokenLinks: []LinkResult{
			{URL: "http://example.com/dead", StatusCode: 404, SourcePage: "http://example.com/"},
			{URL: "http://example.com/fail", Error: "connection refused", SourcePage: "http://example.com/about"},
		},
		Stats: CrawlStats{TotalChecked: 50, BrokenCount: 2, Duration: 5 * time.Second},
	}

	PrintResults(&buf, r)

	got := buf.String()

	// Check header
	if !bytes.Contains([]byte(got), []byte("Broken Links:")) {
		t.Error("missing 'Broken Links:' header")
	}

	// Check first broken link (status code)
	if !bytes.Contains([]byte(got), []byte("URL: http://example.com/dead")) {
		t.Error("missing first broken link URL")
	}
	if !bytes.Contains([]byte(got), []byte("Status: 404")) {
		t.Error("missing status code for first link")
	}
	if !bytes.Contains([]byte(got), []byte("Found on: http://example.com/")) {
		t.Error("missing source page for first link")
	}

	// Check second broken link (error)
	if !bytes.Contains([]byte(got), []byte("URL: http://example.com/fail")) {
		t.Error("missing second broken link URL")
	}
	if !bytes.Contains([]byte(got), []byte("Error: connection refused")) {
		t.Error("missing error for second link")
	}
	if !bytes.Contains([]byte(got), []byte("Found on: http://example.com/about")) {
		t.Error("missing source page for second link")
	}

	// Check summary
	if !bytes.Contains([]byte(got), []byte("Checked 50 URLs, found 2 broken links")) {
		t.Error("missing or incorrect summary line")
	}
}
