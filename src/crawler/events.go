package crawler

import "github.com/lukemcguire/zombiecrawl/result"

// CrawlEvent reports progress for a single checked URL.
type CrawlEvent struct {
	URL           string
	StatusCode    int
	Error         string
	ErrorCategory result.ErrorCategory
	Checked       int
	Broken        int
	IsExternal    bool
}
