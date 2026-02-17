package crawler_test

import (
	"context"
	"errors"
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
		if _, err := fmt.Fprint(w, `<html><body>
			<a href="/page1">Page 1</a>
			<a href="/page2">Page 2</a>
			<a href="https://external.example.com/resource">External</a>
		</body></html>`); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("/page1", func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, `<html><body>
			<a href="/page2">Page 2 again</a>
			<a href="/broken">Broken link</a>
		</body></html>`); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("/page2", func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, `<html><body><p>No links here</p></body></html>`); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("/broken", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})

	return httptest.NewServer(mux)
}

// mustNewCrawler creates a crawler or fails the test.
func mustNewCrawler(t *testing.T, cfg crawler.Config, progressCh chan<- crawler.CrawlEvent) *crawler.Crawler {
	t.Helper()
	c, err := crawler.New(cfg, progressCh)
	if err != nil {
		t.Fatalf("crawler.New() error: %v", err)
	}
	return c
}

// TestCrawlerIntegration verifies the full crawl flow from start URL through
// discovered links, including detection of broken links.
func TestCrawlerIntegration(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	cfg := crawler.Config{
		StartURL:       ts.URL,
		Concurrency:    2,
		RequestTimeout: 5 * time.Second,
	}

	c := mustNewCrawler(t, cfg, nil)
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

// TestCrawlerDeduplication verifies that cyclic link graphs are handled
// correctly without infinite loops or duplicate URL checks.
func TestCrawlerDeduplication(t *testing.T) {
	// Server where every page links to every other page (cycle)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if _, err := fmt.Fprint(w, `<html><body>
			<a href="/a">A</a>
			<a href="/b">B</a>
		</body></html>`); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, `<html><body>
			<a href="/">Home</a>
			<a href="/b">B</a>
			<a href="/a">A self</a>
		</body></html>`); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, `<html><body>
			<a href="/">Home</a>
			<a href="/a">A</a>
		</body></html>`); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := crawler.Config{
		StartURL:       ts.URL,
		Concurrency:    2,
		RequestTimeout: 5 * time.Second,
	}

	c := mustNewCrawler(t, cfg, nil)
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

// TestCrawlerCancellation verifies that the crawler responds correctly to
// context cancellation without goroutine leaks.
func TestCrawlerCancellation(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	cfg := crawler.Config{
		StartURL:       ts.URL,
		Concurrency:    2,
		RequestTimeout: 5 * time.Second,
	}

	c := mustNewCrawler(t, cfg, nil)

	// Cancel context immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	var runErr error
	go func() {
		_, runErr = c.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Good: Run returned without hanging (no goroutine leak)
		// Run() may return nil (graceful early exit) or a context-related error
		if runErr != nil && !errors.Is(runErr, context.Canceled) {
			t.Fatalf("unexpected error: %v", runErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run() did not return after context cancellation (possible goroutine leak)")
	}
}

// newDepthTestServer creates a server with a deep link hierarchy:
// / -> /depth1 -> /depth2 -> /depth3
// Each page also links to an external URL for validation testing.
func newDepthTestServer() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if _, err := fmt.Fprint(w, `<html><body>
			<a href="/depth1">Depth 1</a>
			<a href="https://external.example.com/from-root">External from root</a>
		</body></html>`); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("/depth1", func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, `<html><body>
			<a href="/depth2">Depth 2</a>
			<a href="https://external.example.com/from-depth1">External from depth1</a>
		</body></html>`); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("/depth2", func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, `<html><body>
			<a href="/depth3">Depth 3</a>
			<a href="https://external.example.com/from-depth2">External from depth2</a>
		</body></html>`); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("/depth3", func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, `<html><body>
			<a href="https://external.example.com/from-depth3">External from depth3</a>
		</body></html>`); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	return httptest.NewServer(mux)
}

// TestCrawlerMaxDepthLimitsInternalCrawling verifies that MaxDepth restricts
// crawling to the specified depth while still validating external links.
func TestCrawlerMaxDepthLimitsInternalCrawling(t *testing.T) {
	ts := newDepthTestServer()
	defer ts.Close()

	cfg := crawler.Config{
		StartURL:       ts.URL,
		Concurrency:    2,
		RequestTimeout: 5 * time.Second,
		MaxDepth:       1, // Only / (depth 0) and /depth1 (depth 1)
	}

	c := mustNewCrawler(t, cfg, nil)
	result, err := c.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	// With MaxDepth=1, should only crawl:
	// - / (depth 0, start URL)
	// - /depth1 (depth 1)
	// Should NOT crawl /depth2 or /depth3 (would be depth 2 and 3)
	// External links from crawled pages are still validated (not crawled)
	// Expected total: 2 internal + 2 external = 4
	if result.Stats.TotalChecked != 4 {
		t.Errorf("expected 4 URLs checked (2 internal + 2 external), got %d", result.Stats.TotalChecked)
	}
}

// TestCrawlerMaxDepthZeroMeansUnlimited verifies that MaxDepth=0 allows
// unlimited depth crawling.
func TestCrawlerMaxDepthZeroMeansUnlimited(t *testing.T) {
	ts := newDepthTestServer()
	defer ts.Close()

	cfg := crawler.Config{
		StartURL:       ts.URL,
		Concurrency:    2,
		RequestTimeout: 5 * time.Second,
		MaxDepth:       0, // unlimited (default)
	}

	c := mustNewCrawler(t, cfg, nil)
	result, err := c.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	// With MaxDepth=0 (unlimited), should crawl all 4 internal pages
	// and validate all 4 external links = 8 total
	if result.Stats.TotalChecked != 8 {
		t.Errorf("expected 8 URLs checked (4 internal + 4 external), got %d", result.Stats.TotalChecked)
	}
}
