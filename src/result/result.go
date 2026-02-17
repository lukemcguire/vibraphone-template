// Package result provides types and output writers for crawl results.
package result

import "time"

// LinkResult represents the result of checking a single link.
type LinkResult struct {
	URL           string        `json:"url"`                   // The URL that was checked
	StatusCode    int           `json:"status_code,omitempty"` // HTTP status code (0 if unreachable)
	Error         string        `json:"error,omitempty"`       // Error message if the check failed
	ErrorCategory ErrorCategory `json:"error_type,omitempty"`  // Category classification of the error
	SourcePage    string        `json:"source_page"`           // The page where this link was found
	IsExternal    bool          `json:"is_external"`           // Whether this link points outside the crawled domain
}

// CrawlStats contains aggregate statistics for a crawl operation.
type CrawlStats struct {
	TotalChecked int           `json:"total_checked"` // Total number of links checked
	BrokenCount  int           `json:"broken_count"`  // Number of broken links found
	Duration     time.Duration `json:"duration"`      // Total time taken for the crawl
}

// Result represents the complete output of a broken link crawl.
type Result struct {
	BrokenLinks []LinkResult `json:"broken_links"` // All broken links discovered
	Stats       CrawlStats   `json:"stats"`        // Aggregate statistics
}
