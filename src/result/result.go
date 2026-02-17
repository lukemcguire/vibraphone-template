package result

import "time"

// LinkResult represents the result of checking a single link.
type LinkResult struct {
	URL           string        // The URL that was checked
	StatusCode    int           // HTTP status code (0 if unreachable)
	Error         string        // Error message if the check failed
	ErrorCategory ErrorCategory // Category classification of the error
	SourcePage    string        // The page where this link was found
	IsExternal    bool          // Whether this link points outside the crawled domain
}

// CrawlStats contains aggregate statistics for a crawl operation.
type CrawlStats struct {
	TotalChecked int           // Total number of links checked
	BrokenCount  int           // Number of broken links found
	Duration     time.Duration // Total time taken for the crawl
}

// Result represents the complete output of a broken link crawl.
type Result struct {
	BrokenLinks []LinkResult // All broken links discovered
	Stats       CrawlStats   // Aggregate statistics
}
