package crawler

import (
	"context"
	"net/http"
	"time"

	"github.com/lukemcguire/zombiecrawl/result"
	"github.com/lukemcguire/zombiecrawl/urlutil"
)

// Config holds crawler configuration.
type Config struct {
	StartURL       string        // The starting URL for the crawl
	Concurrency    int           // Number of concurrent workers (default 17)
	RequestTimeout time.Duration // Per-request timeout (default 10s)
}

// CrawlJob represents a URL to be checked.
type CrawlJob struct {
	URL        string // The URL to check
	SourcePage string // The page where this link was found
	IsExternal bool   // Whether this is an external link (validate only, don't crawl)
}

// CrawlResult represents the result of checking a URL.
type CrawlResult struct {
	Job    CrawlJob          // The original job
	Links  []string          // Discovered links (internal pages only)
	Result *result.LinkResult // Broken link info (if broken)
	Err    error             // Any error that occurred
}

// CheckURL fetches a URL and returns the result.
// For external links: HEAD request first, fall back to GET if HEAD fails.
// For internal links: GET request (need body for link extraction).
func CheckURL(ctx context.Context, client *http.Client, job CrawlJob, cfg Config) CrawlResult {
	// Create per-request context with timeout
	reqCtx, cancel := context.WithTimeout(ctx, cfg.RequestTimeout)
	defer cancel()

	var resp *http.Response
	var err error

	if job.IsExternal {
		// External link: try HEAD first
		req, reqErr := http.NewRequestWithContext(reqCtx, http.MethodHead, job.URL, nil)
		if reqErr != nil {
			return CrawlResult{
				Job: job,
				Result: &result.LinkResult{
					URL:        job.URL,
					SourcePage: job.SourcePage,
					IsExternal: true,
					Error:      reqErr.Error(),
				},
			}
		}

		resp, err = client.Do(req)
		if err != nil {
			return CrawlResult{
				Job: job,
				Result: &result.LinkResult{
					URL:        job.URL,
					SourcePage: job.SourcePage,
					IsExternal: true,
					Error:      err.Error(),
				},
			}
		}
		defer func() { _ = resp.Body.Close() }()

		// If HEAD returns 405 Method Not Allowed, fall back to GET
		if resp.StatusCode == http.StatusMethodNotAllowed {
			getReq, getErr := http.NewRequestWithContext(reqCtx, http.MethodGet, job.URL, nil)
			if getErr != nil {
				return CrawlResult{
					Job: job,
					Result: &result.LinkResult{
						URL:        job.URL,
						SourcePage: job.SourcePage,
						IsExternal: true,
						Error:      getErr.Error(),
					},
				}
			}
			resp, err = client.Do(getReq)
			if err != nil {
				return CrawlResult{
					Job: job,
					Result: &result.LinkResult{
						URL:        job.URL,
						SourcePage: job.SourcePage,
						IsExternal: true,
						Error:      err.Error(),
					},
				}
			}
			defer func() { _ = resp.Body.Close() }()
		}

		// Check status for external link
		status := resp.StatusCode
		if status >= 400 {
			return CrawlResult{
				Job: job,
				Result: &result.LinkResult{
					URL:        job.URL,
					StatusCode: status,
					SourcePage: job.SourcePage,
					IsExternal: true,
				},
			}
		}

		// External link is valid
		return CrawlResult{Job: job}
	}

	// Internal link: GET request
	req, reqErr := http.NewRequestWithContext(reqCtx, http.MethodGet, job.URL, nil)
	if reqErr != nil {
		return CrawlResult{
			Job: job,
			Result: &result.LinkResult{
				URL:        job.URL,
				SourcePage: job.SourcePage,
				IsExternal: false,
				Error:      reqErr.Error(),
			},
		}
	}

	resp, err = client.Do(req)
	if err != nil {
		return CrawlResult{
			Job: job,
			Result: &result.LinkResult{
				URL:        job.URL,
				SourcePage: job.SourcePage,
				IsExternal: false,
				Error:      err.Error(),
			},
		}
	}
	defer func() { _ = resp.Body.Close() }()

	status := resp.StatusCode
	if status >= 400 {
		return CrawlResult{
			Job: job,
			Result: &result.LinkResult{
				URL:        job.URL,
				StatusCode: status,
				SourcePage: job.SourcePage,
				IsExternal: false,
			},
		}
	}

	// Extract links from the response body
	links, err := ExtractLinks(resp.Body, resp.Request.URL)
	if err != nil {
		// Non-fatal: just return empty links
		return CrawlResult{Job: job, Links: []string{}}
	}

	// Filter links: separate internal and external
	var internalLinks []string
	baseHost := resp.Request.URL.Host
	for _, link := range links {
		if urlutil.IsSameDomain(link, baseHost) {
			internalLinks = append(internalLinks, link)
		}
	}

	return CrawlResult{Job: job, Links: internalLinks}
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig(startURL string) Config {
	return Config{
		StartURL:       startURL,
		Concurrency:    17,
		RequestTimeout: 10 * time.Second,
	}
}
