package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lukemcguire/zombiecrawl/crawler"
	"github.com/lukemcguire/zombiecrawl/result"
)

// CrawlProgressMsg reports progress for a single checked URL.
type CrawlProgressMsg struct {
	Checked int
	Broken  int
	URL     string
}

// CrawlDoneMsg signals the crawl has completed.
type CrawlDoneMsg struct {
	Result *result.Result
	Err    error
}

// waitForProgress returns a tea.Cmd that reads one event from the progress
// channel. When the channel closes, it returns a CrawlDoneMsg with nil Result
// (the actual result comes from startCrawl).
func waitForProgress(ch <-chan crawler.CrawlEvent) tea.Cmd {
	return func() tea.Msg {
		evt, ok := <-ch
		if !ok {
			return CrawlDoneMsg{}
		}
		return CrawlProgressMsg{
			Checked: evt.Checked,
			Broken:  evt.Broken,
			URL:     evt.URL,
		}
	}
}
