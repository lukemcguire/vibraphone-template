package crawler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/temoto/robotstxt"
)

// cachedRobots stores parsed robots.txt data with fetch timestamp.
type cachedRobots struct {
	data      *robotstxt.RobotsData
	fetchedAt time.Time
}

// RobotsChecker fetches and caches robots.txt rules per host.
type RobotsChecker struct {
	client   *http.Client
	cache    sync.Map // host string -> *cachedRobots
	cacheTTL time.Duration
}

// NewRobotsChecker creates a RobotsChecker with the given HTTP client.
func NewRobotsChecker(client *http.Client) *RobotsChecker {
	return &RobotsChecker{
		client:   client,
		cacheTTL: time.Hour, // 1-hour cache TTL
	}
}

// Allowed checks if the given URL is allowed to be crawled by the user agent.
// Returns true if allowed, false if disallowed by robots.txt.
// Errors (network, parsing) result in allow-all behavior.
func (r *RobotsChecker) Allowed(ctx context.Context, rawURL, userAgent string) (bool, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		// Invalid URL - allow by default
		return true, fmt.Errorf("parse URL: %w", err)
	}

	host := parsedURL.Host
	if host == "" {
		return true, nil
	}

	// Check cache for valid entry
	if cached, ok := r.cache.Load(host); ok {
		cachedEntry, ok := cached.(*cachedRobots)
		if !ok || cachedEntry == nil {
			// Invalid cache entry - treat as miss and refetch
			r.cache.Delete(host)
		} else if time.Since(cachedEntry.fetchedAt) < r.cacheTTL {
			// Cache hit and valid TTL
			if cachedEntry.data == nil {
				// Nil data means allow-all (404, 5xx, or fetch error)
				return true, nil
			}
			// Check against robots.txt rules
			return cachedEntry.data.TestAgent(parsedURL.Path, userAgent), nil
		}
	}

	// Cache miss or expired - fetch robots.txt
	robotsURL := fmt.Sprintf("%s://%s/robots.txt", parsedURL.Scheme, host)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL, nil)
	if err != nil {
		r.cacheNilEntry(host)
		return true, fmt.Errorf("create robots.txt request for host %s: %w", host, err)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		// Network error (timeout, connection refused, etc.) - allow all
		r.cacheNilEntry(host)
		return true, fmt.Errorf("fetch robots.txt for host %s: %w", host, err)
	}

	body, readErr := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	// Combine read and close errors, prioritizing read error
	if readErr != nil {
		r.cacheNilEntry(host)
		if closeErr != nil {
			return true, fmt.Errorf("read robots.txt body for host %s: %w (close error: %v)", host, readErr, closeErr)
		}
		return true, fmt.Errorf("read robots.txt body for host %s: %w", host, readErr)
	}
	if closeErr != nil {
		r.cacheNilEntry(host)
		return true, fmt.Errorf("close robots.txt response body for host %s: %w", host, closeErr)
	}

	// Handle status codes that should allow all crawling
	// 404: robots.txt doesn't exist - allow all
	// 5xx: server error - allow all (fail open)
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode >= 500 {
		r.cacheNilEntry(host)
		return true, nil
	}

	// FromStatusAndBytes handles status codes:
	// - 2xx: parse and apply rules
	robots, err := robotstxt.FromStatusAndBytes(resp.StatusCode, body)
	if err != nil {
		// Parse error - cache nil and allow
		r.cacheNilEntry(host)
		return true, fmt.Errorf("parse robots.txt for host %s: %w", host, err)
	}

	if robots == nil {
		// 404, 5xx, or other allow-all status
		r.cacheNilEntry(host)
		return true, nil
	}

	// Cache the parsed robots.txt
	r.cache.Store(host, &cachedRobots{
		data:      robots,
		fetchedAt: time.Now(),
	})

	return robots.TestAgent(parsedURL.Path, userAgent), nil
}

// cacheNilEntry stores a nil entry to indicate allow-all for this host.
func (r *RobotsChecker) cacheNilEntry(host string) {
	r.cache.Store(host, &cachedRobots{
		data:      nil,
		fetchedAt: time.Now(),
	})
}

// ClearCache removes all cached robots.txt entries.
// Useful for testing.
func (r *RobotsChecker) ClearCache() {
	r.cache = sync.Map{}
}
