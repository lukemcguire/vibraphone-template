package crawler_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lukemcguire/zombiecrawl/crawler"
)

// newTestServer creates an httptest server with a multi-page site for integration testing.
// Site structure:
//
//	/        -> links to /page1, /page2, external
//	/page1   -> links to /page2 (dedup), /broken
//	/page2   -> no outgoing links
//	/broken  -> 404
func newTestServer() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprint(w, `<html><body>
			<a href="/page1">Page 1</a>
			<a href="/page2">Page 2</a>
			<a href="https://external.example.com/resource">External</a>
		</body></html>`)
	})

	mux.HandleFunc("/page1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<html><body>
			<a href="/page2">Page 2 again</a>
			<a href="/broken">Broken link</a>
		</body></html>`)
	})

	mux.HandleFunc("/page2", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<html><body><p>No links here</p></body></html>`)
	})

	mux.HandleFunc("/broken", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})

	return httptest.NewServer(mux)
}

func TestCrawlerIntegration(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	cfg := crawler.Config{
		StartURL:       ts.URL,
		Concurrency:    2,
		RequestTimeout: 5 * time.Second,
	}

	c := crawler.New(cfg, nil)
	result, err := c.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	// Should find broken links: /broken (404) and external (DNS failure).
	// Verify /broken with 404 is among them.
	var found404 bool
	for _, bl := range result.BrokenLinks {
		if strings.HasSuffix(bl.URL, "/broken") && bl.StatusCode == 404 {
			found404 = true
		}
	}
	if !found404 {
		t.Error("expected /broken with status 404 among broken links")
		for _, bl := range result.BrokenLinks {
			t.Logf("  broken: %s (status=%d, err=%s)", bl.URL, bl.StatusCode, bl.Error)
		}
	}

	// TotalChecked: /, /page1, /page2, /broken (4 internal) + 1 external = 5.
	if result.Stats.TotalChecked != 5 {
		t.Errorf("expected 5 URLs checked, got %d", result.Stats.TotalChecked)
	}
}

func TestCrawlerDeduplication(t *testing.T) {
	// Server where every page links to every other page (cycle)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprint(w, `<html><body>
			<a href="/a">A</a>
			<a href="/b">B</a>
		</body></html>`)
	})
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<html><body>
			<a href="/">Home</a>
			<a href="/b">B</a>
			<a href="/a">A self</a>
		</body></html>`)
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<html><body>
			<a href="/">Home</a>
			<a href="/a">A</a>
		</body></html>`)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := crawler.Config{
		StartURL:       ts.URL,
		Concurrency:    2,
		RequestTimeout: 5 * time.Second,
	}

	c := crawler.New(cfg, nil)
	result, err := c.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	// Should have no broken links
	if len(result.BrokenLinks) != 0 {
		t.Errorf("expected 0 broken links, got %d", len(result.BrokenLinks))
	}

	// Should check exactly 3 URLs: /, /a, /b (no duplicates)
	if result.Stats.TotalChecked != 3 {
		t.Errorf("expected exactly 3 URLs checked (dedup), got %d", result.Stats.TotalChecked)
	}
}

func TestCrawlerCancellation(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	cfg := crawler.Config{
		StartURL:       ts.URL,
		Concurrency:    2,
		RequestTimeout: 5 * time.Second,
	}

	c := crawler.New(cfg, nil)

	// Cancel context immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		_, _ = c.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Good: Run returned without hanging
	case <-time.After(3 * time.Second):
		t.Fatal("Run() did not return after context cancellation (possible goroutine leak)")
	}
}
