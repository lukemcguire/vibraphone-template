package crawler

// CrawlEvent reports progress for a single checked URL.
type CrawlEvent struct {
	URL        string
	StatusCode int
	Error      string
	Checked    int
	Broken     int
	IsExternal bool
}
