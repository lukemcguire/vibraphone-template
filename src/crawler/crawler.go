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
	cfg        Config
	client     *http.Client
	limiter    *rate.Limiter
	visited    sync.Map
	results    []result.LinkResult
	mu         sync.Mutex
	total      int
	progressCh chan<- CrawlEvent
}

// New creates a Crawler with the given configuration.
// The progressCh parameter is optional; pass nil to disable progress events.
func New(cfg Config, progressCh chan<- CrawlEvent) *Crawler {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 17
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

	limiter := rate.NewLimiter(rate.Limit(cfg.RateLimit), cfg.RateLimit)

	return &Crawler{
		cfg:        cfg,
		client:     &http.Client{},
		limiter:    limiter,
		progressCh: progressCh,
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

	var wg sync.WaitGroup

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
					cr := CheckURL(groupCtx, c.client, job, c.cfg)
					// Always send result - coordinator must receive it to call wg.Done()
					results <- cr
				case <-groupCtx.Done():
					// Drain remaining jobs to ensure wg.Done() is called for each
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

	// Seed the first job.
	wg.Add(1)
	jobs <- CrawlJob{URL: startURL, SourcePage: "", IsExternal: false}

	// Close results channel when all work is done (managed via errgroup)
	errGroup.Go(func() error {
		wg.Wait()
		close(results)
		return nil
	})

	// Coordinator: read results, enqueue discovered links.
	// Process all results until channel closes - workers always send results
	// so we don't need special cancellation handling here.
	for cr := range results {
		c.mu.Lock()
		c.total++
		c.mu.Unlock()

		if cr.Result != nil {
			c.mu.Lock()
			c.results = append(c.results, *cr.Result)
			c.mu.Unlock()
		}

		if c.progressCh != nil {
			evt := CrawlEvent{
				URL:        cr.Job.URL,
				IsExternal: cr.Job.IsExternal,
				Checked:    c.total,
			}
			if cr.Result != nil {
				evt.StatusCode = cr.Result.StatusCode
				evt.Error = cr.Result.Error
				c.mu.Lock()
				evt.Broken = len(c.results)
				c.mu.Unlock()
			} else if cr.Err != nil {
				evt.Error = cr.Err.Error()
			}
			c.progressCh <- evt
		}

		// Enqueue discovered links from internal pages (skip if context cancelled)
		if !cr.Job.IsExternal && ctx.Err() == nil {
			startHost := hostFromURL(startURL)
			for _, link := range cr.Links {
				normalized, normErr := urlutil.Normalize(link)
				if normErr != nil {
					continue
				}
				if _, loaded := c.visited.LoadOrStore(normalized, true); loaded {
					continue
				}
				isExternal := !urlutil.IsSameDomain(normalized, startHost)
				wg.Add(1)
				jobs <- CrawlJob{
					URL:        normalized,
					SourcePage: cr.Job.URL,
					IsExternal: isExternal,
				}
			}
		}

		wg.Done()
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
