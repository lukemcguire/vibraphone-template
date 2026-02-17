package crawler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/lukemcguire/zombiecrawl/result"
)

// isBinaryContentType returns true if the content type indicates a binary file
// that should not be parsed for links (images, PDFs, videos, audio, archives, fonts).
func isBinaryContentType(contentType string) bool {
	// Normalize: lowercase and strip charset/parameters
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if idx := strings.Index(contentType, ";"); idx != -1 {
		contentType = strings.TrimSpace(contentType[:idx])
	}

	// Check image types
	if strings.HasPrefix(contentType, "image/") {
		return true
	}

	// Check video types
	if strings.HasPrefix(contentType, "video/") {
		return true
	}

	// Check audio types
	if strings.HasPrefix(contentType, "audio/") {
		return true
	}

	// Check font types
	if strings.HasPrefix(contentType, "font/") {
		return true
	}

	// Check specific binary application types
	binaryTypes := []string{
		"application/pdf",
		"application/zip",
		"application/x-zip-compressed",
		"application/gzip",
		"application/vnd.rar",
		"application/x-7z-compressed",
		"application/octet-stream",
	}

	for _, bt := range binaryTypes {
		if contentType == bt {
			return true
		}
	}

	return false
}

// Config holds crawler configuration.
type Config struct {
	StartURL       string        // The starting URL for the crawl
	Concurrency    int           // Number of concurrent workers (default 17)
	RequestTimeout time.Duration // Per-request timeout (default 10s)
	RateLimit      int           // Requests per second (default 10)
	UserAgent      string        // HTTP User-Agent header (default "zombiecrawl/1.0")
	RetryPolicy    RetryPolicy   // Retry policy for failed requests
	MaxDepth       int           // Maximum crawl depth (0 = unlimited)
}

// CrawlJob represents a URL to be checked.
type CrawlJob struct {
	URL        string // The URL to check
	SourcePage string // The page where this link was found
	IsExternal bool   // Whether this is an external link (validate only, don't crawl)
	Depth      int    // Current crawl depth (0 = start URL)
}

// CrawlResult represents the result of checking a URL.
type CrawlResult struct {
	Job    CrawlJob           // The original job
	Links  []string           // Discovered links (internal pages only)
	Result *result.LinkResult // Broken link info (if broken)
	Err    error              // Any error that occurred
}

// CheckURL fetches a URL and returns the result.
// For external links: HEAD request first, fall back to GET if HEAD fails.
// For internal links: GET request (need body for link extraction).
func CheckURL(ctx context.Context, client *http.Client, job CrawlJob, cfg Config) (res CrawlResult) {
	res.Job = job

	// Create per-request context with timeout
	reqCtx, cancel := context.WithTimeout(ctx, cfg.RequestTimeout)
	defer cancel()

	// Track redirect loop detection
	var isRedirectLoop bool
	var visitedInChain []string

	// Create per-request client with redirect loop detection
	loopClient := &http.Client{
		Timeout: cfg.RequestTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			currentURL := req.URL.String()

			// Check if we've seen this URL in the current chain
			for _, visitedURL := range visitedInChain {
				if visitedURL == currentURL {
					isRedirectLoop = true
					return http.ErrUseLastResponse
				}
			}
			visitedInChain = append(visitedInChain, currentURL)

			// Also limit total redirects (10 is Go default)
			if len(via) >= 10 {
				isRedirectLoop = true
				return errors.New("stopped after 10 redirects")
			}
			return nil
		},
	}

	var resp *http.Response
	var err error

	if job.IsExternal {
		// External link: try HEAD first
		req, reqErr := http.NewRequestWithContext(reqCtx, http.MethodHead, job.URL, nil)
		if reqErr != nil {
			res.Result = &result.LinkResult{
				URL:           job.URL,
				SourcePage:    job.SourcePage,
				IsExternal:    true,
				Error:         reqErr.Error(),
				ErrorCategory: result.ClassifyError(reqErr, 0, false),
			}
			return
		}

		resp, err = loopClient.Do(req)
		if err != nil {
			cat := result.ClassifyError(err, 0, isRedirectLoop)
			res.Result = &result.LinkResult{
				URL:           job.URL,
				SourcePage:    job.SourcePage,
				IsExternal:    true,
				Error:         err.Error(),
				ErrorCategory: cat,
			}
			return
		}
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil && res.Err == nil {
				res.Err = fmt.Errorf("close response body: %w", closeErr)
			}
		}()

		// If HEAD returns 405 Method Not Allowed, fall back to GET
		if resp.StatusCode == http.StatusMethodNotAllowed {
			getReq, getErr := http.NewRequestWithContext(reqCtx, http.MethodGet, job.URL, nil)
			if getErr != nil {
				res.Result = &result.LinkResult{
					URL:           job.URL,
					SourcePage:    job.SourcePage,
					IsExternal:    true,
					Error:         getErr.Error(),
					ErrorCategory: result.ClassifyError(getErr, 0, false),
				}
				return
			}
			// Reset loop detection for new request
			isRedirectLoop = false
			visitedInChain = nil
			resp, err = loopClient.Do(getReq)
			if err != nil {
				cat := result.ClassifyError(err, 0, isRedirectLoop)
				res.Result = &result.LinkResult{
					URL:           job.URL,
					SourcePage:    job.SourcePage,
					IsExternal:    true,
					Error:         err.Error(),
					ErrorCategory: cat,
				}
				return
			}
			defer func() {
				if closeErr := resp.Body.Close(); closeErr != nil && res.Err == nil {
					res.Err = fmt.Errorf("close response body: %w", closeErr)
				}
			}()
		}

		// Check status for external link
		status := resp.StatusCode
		if status >= 400 || isRedirectLoop {
			errMsg := ""
			if isRedirectLoop {
				errMsg = "redirect loop detected"
			}
			res.Result = &result.LinkResult{
				URL:           job.URL,
				StatusCode:    status,
				SourcePage:    job.SourcePage,
				IsExternal:    true,
				Error:         errMsg,
				ErrorCategory: result.ClassifyError(nil, status, isRedirectLoop),
			}
			return
		}

		// External link is valid
		return
	}

	// Internal link: GET request
	req, reqErr := http.NewRequestWithContext(reqCtx, http.MethodGet, job.URL, nil)
	if reqErr != nil {
		res.Result = &result.LinkResult{
			URL:           job.URL,
			SourcePage:    job.SourcePage,
			IsExternal:    false,
			Error:         reqErr.Error(),
			ErrorCategory: result.ClassifyError(reqErr, 0, false),
		}
		return
	}

	resp, err = loopClient.Do(req)
	if err != nil {
		cat := result.ClassifyError(err, 0, isRedirectLoop)
		res.Result = &result.LinkResult{
			URL:           job.URL,
			SourcePage:    job.SourcePage,
			IsExternal:    false,
			Error:         err.Error(),
			ErrorCategory: cat,
		}
		return
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && res.Err == nil {
			res.Err = fmt.Errorf("close response body: %w", closeErr)
		}
	}()

	status := resp.StatusCode
	if status >= 400 || isRedirectLoop {
		errMsg := ""
		if isRedirectLoop {
			errMsg = "redirect loop detected"
		}
		res.Result = &result.LinkResult{
			URL:           job.URL,
			StatusCode:    status,
			SourcePage:    job.SourcePage,
			IsExternal:    false,
			Error:         errMsg,
			ErrorCategory: result.ClassifyError(nil, status, isRedirectLoop),
		}
		return
	}

	// Check if this is a binary content type - skip parsing if so
	contentType := resp.Header.Get("Content-Type")
	if isBinaryContentType(contentType) {
		// Binary files are valid but have no links to extract
		res.Links = []string{}
		return
	}

	// Extract links from the response body
	links, extractErr := ExtractLinks(resp.Body, resp.Request.URL)
	if extractErr != nil {
		res.Err = fmt.Errorf("extract links from %s: %w", job.URL, extractErr)
		res.Links = []string{}
		return
	}

	res.Links = links
	return
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig(startURL string) Config {
	return Config{
		StartURL:       startURL,
		Concurrency:    17,
		RequestTimeout: 10 * time.Second,
		RateLimit:      10,
		UserAgent:      "zombiecrawl/1.0 (+https://github.com/lukemcguire/zombiecrawl)",
		RetryPolicy:    DefaultRetryPolicy(),
		MaxDepth:       0, // unlimited
	}
}
