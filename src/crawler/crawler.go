package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/lukemcguire/zombiecrawl/result"
	"github.com/lukemcguire/zombiecrawl/urlutil"
)

// Crawler coordinates BFS link checking with a concurrent worker pool.
type Crawler struct {
	cfg        Config
	client     *http.Client
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

	return &Crawler{
		cfg:        cfg,
		client:     &http.Client{},
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
	if u, parseErr := url.Parse(startURL); parseErr == nil && u.Path == "" {
		u.Path = "/"
		startURL = u.String()
	}

	jobs := make(chan CrawlJob, c.cfg.Concurrency*3)
	results := make(chan CrawlResult, c.cfg.Concurrency*3)

	var wg sync.WaitGroup

	// Mark start URL as visited before enqueueing.
	c.visited.Store(startURL, true)

	// Launch workers.
	for i := 0; i < c.cfg.Concurrency; i++ {
		go func() {
			for {
				select {
				case job, ok := <-jobs:
					if !ok {
						return
					}
					cr := CheckURL(ctx, c.client, job, c.cfg)
					results <- cr
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// Seed the first job.
	wg.Add(1)
	jobs <- CrawlJob{URL: startURL, SourcePage: "", IsExternal: false}

	// Close results channel when all work is done.
	go func() {
		wg.Wait()
		close(results)
	}()

	// Coordinator: read results, enqueue discovered links.
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

		// Enqueue discovered links from internal pages.
		if !cr.Job.IsExternal {
			startHost := hostFromURL(startURL)
			for _, link := range cr.Links {
				normalized, nerr := urlutil.Normalize(link)
				if nerr != nil {
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
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.Hostname()
}
