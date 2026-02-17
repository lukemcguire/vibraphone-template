// Package crawler provides a concurrent web crawler for discovering broken links.
// It implements BFS crawling with robots.txt compliance, rate limiting, and
// progress event streaming.
package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"

	"github.com/lukemcguire/zombiecrawl/result"
	"github.com/lukemcguire/zombiecrawl/urlutil"
)

// Crawler coordinates BFS link checking with a concurrent worker pool.
type Crawler struct {
	cfg           Config
	client        *http.Client
	limiter       *rate.Limiter
	robotsChecker *RobotsChecker
	visited       sync.Map
	results       []result.LinkResult
	mu            sync.Mutex
	total         int
	progressCh    chan<- CrawlEvent
}

// New creates a Crawler with the given configuration.
// The progressCh parameter is optional; pass nil to disable progress events.
func New(cfg Config, progressCh chan<- CrawlEvent) *Crawler {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 10
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 10 * time.Second
	}
	if cfg.RateLimit <= 0 {
		cfg.RateLimit = 10
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "zombiecrawl/1.0 (+https://github.com/lukemcguire/zombiecrawl)"
	}
	if cfg.RetryPolicy.MaxRetries == 0 {
		cfg.RetryPolicy = DefaultRetryPolicy()
	}

	limiter := rate.NewLimiter(rate.Limit(cfg.RateLimit), cfg.RateLimit)

	// Separate client for robots.txt with shorter timeout
	robotsClient := &http.Client{Timeout: 5 * time.Second}

	return &Crawler{
		cfg:           cfg,
		client:        &http.Client{},
		limiter:       limiter,
		robotsChecker: NewRobotsChecker(robotsClient),
		progressCh:    progressCh,
	}
}

// Run executes the crawl starting from cfg.StartURL and returns broken link results.
func (c *Crawler) Run(ctx context.Context) (*result.Result, error) {
	start := time.Now()

	startURL, err := urlutil.Normalize(c.cfg.StartURL)
	if err != nil {
		return nil, fmt.Errorf("normalize start URL: %w", err)
	}

	// Ensure root path consistency: "http://host" and "http://host/" must dedup.
	if parsedURL, parseErr := url.Parse(startURL); parseErr == nil && parsedURL.Path == "" {
		parsedURL.Path = "/"
		startURL = parsedURL.String()
	}

	jobs := make(chan CrawlJob, c.cfg.Concurrency*3)
	results := make(chan CrawlResult, c.cfg.Concurrency*3)

	var pendingJobs sync.WaitGroup

	// Mark start URL as visited before enqueueing.
	c.visited.Store(startURL, true)

	// Use errgroup for structured goroutine management
	errGroup, groupCtx := errgroup.WithContext(ctx)

	// Launch workers with errgroup
	for range c.cfg.Concurrency {
		errGroup.Go(func() error {
			for {
				select {
				case job, ok := <-jobs:
					if !ok {
						return nil
					}
					// Wait for rate limiter before making request
					if waitErr := c.limiter.Wait(groupCtx); waitErr != nil {
						// Context cancelled while waiting - must still send result to unblock coordinator
						results <- CrawlResult{Job: job}
						return fmt.Errorf("rate limiter wait: %w", waitErr)
					}
					crawlResult := CheckURLWithRetry(groupCtx, c.client, job, c.cfg, c.cfg.RetryPolicy)
					// Always send result - coordinator must receive it to call pendingJobs.Done()
					results <- crawlResult
				case <-groupCtx.Done():
					// Drain remaining jobs to ensure pendingJobs.Done() is called for each
					// Use non-blocking receive to avoid blocking forever
					for {
						select {
						case job, ok := <-jobs:
							if !ok {
								return nil
							}
							// Send synthetic result for unprocessed job
							results <- CrawlResult{Job: job}
						default:
							// No more jobs to drain
							return nil
						}
					}
				}
			}
		})
	}

	// Check robots.txt for start URL before seeding the first job.
	// Errors are treated as allow-all (fail-open) but we surface them via progress channel.
	allowed, robotsErr := c.robotsChecker.Allowed(ctx, startURL, c.cfg.UserAgent)
	if robotsErr != nil && c.progressCh != nil {
		c.progressCh <- CrawlEvent{
			URL:        startURL,
			Error:      fmt.Sprintf("robots.txt check: %v", robotsErr),
			IsExternal: false,
		}
	}
	if !allowed {
		return nil, fmt.Errorf("start URL %s is disallowed by robots.txt", startURL)
	}

	// Seed the first job.
	pendingJobs.Add(1)
	jobs <- CrawlJob{URL: startURL, SourcePage: "", IsExternal: false}

	// Close results channel when all work is done (managed via errgroup)
	errGroup.Go(func() error {
		pendingJobs.Wait()
		close(results)
		return nil
	})

	// Coordinator: read results, enqueue discovered links.
	// Process all results until channel closes - workers always send results
	// so we don't need special cancellation handling here.
	for crawlResult := range results {
		c.mu.Lock()
		c.total++
		c.mu.Unlock()

		if crawlResult.Result != nil {
			c.mu.Lock()
			c.results = append(c.results, *crawlResult.Result)
			c.mu.Unlock()
		}

		if c.progressCh != nil {
			evt := CrawlEvent{
				URL:        crawlResult.Job.URL,
				IsExternal: crawlResult.Job.IsExternal,
				Checked:    c.total,
			}
			if crawlResult.Result != nil {
				evt.StatusCode = crawlResult.Result.StatusCode
				evt.Error = crawlResult.Result.Error
				c.mu.Lock()
				evt.Broken = len(c.results)
				c.mu.Unlock()
			} else if crawlResult.Err != nil {
				evt.Error = crawlResult.Err.Error()
			}
			c.progressCh <- evt
		}

		// Enqueue discovered links from internal pages (skip if context cancelled)
		if !crawlResult.Job.IsExternal && ctx.Err() == nil {
			startHost := hostFromURL(startURL)
			for _, link := range crawlResult.Links {
				normalized, normErr := urlutil.Normalize(link)
				if normErr != nil {
					// Surface normalization errors via progress channel
					if c.progressCh != nil {
						c.progressCh <- CrawlEvent{
							URL:        link,
							Error:      fmt.Sprintf("normalize URL: %v", normErr),
							IsExternal: false,
						}
					}
					continue
				}
				if _, loaded := c.visited.LoadOrStore(normalized, true); loaded {
					continue
				}
				// Check robots.txt before enqueueing.
				// Errors are treated as allow-all (fail-open) but we surface them via progress channel.
				allowed, robotsErr := c.robotsChecker.Allowed(ctx, normalized, c.cfg.UserAgent)
				if robotsErr != nil && c.progressCh != nil {
					c.progressCh <- CrawlEvent{
						URL:        normalized,
						Error:      fmt.Sprintf("robots.txt check: %v", robotsErr),
						IsExternal: false,
					}
				}
				if !allowed {
					continue // Skip disallowed URLs
				}
				isExternal := !urlutil.IsSameDomain(normalized, startHost)
				pendingJobs.Add(1)
				jobs <- CrawlJob{
					URL:        normalized,
					SourcePage: crawlResult.Job.URL,
					IsExternal: isExternal,
				}
			}
		}

		pendingJobs.Done()
	}

	close(jobs)

	// Wait for all goroutines to complete
	if waitErr := errGroup.Wait(); waitErr != nil {
		return nil, fmt.Errorf("wait for workers: %w", waitErr)
	}

	c.mu.Lock()
	brokenLinks := make([]result.LinkResult, len(c.results))
	copy(brokenLinks, c.results)
	totalChecked := c.total
	c.mu.Unlock()

	return &result.Result{
		BrokenLinks: brokenLinks,
		Stats: result.CrawlStats{
			TotalChecked: totalChecked,
			BrokenCount:  len(brokenLinks),
			Duration:     time.Since(start),
		},
	}, nil
}

// hostFromURL extracts the hostname (without port) from a URL string.
// This matches what urlutil.IsSameDomain expects for comparison.
func hostFromURL(rawURL string) string {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return parsedURL.Hostname()
}
