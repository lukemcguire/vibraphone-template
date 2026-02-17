// Package tui provides the Bubble Tea terminal UI for zombiecrawl,
// displaying live crawl progress and a styled summary of results.
package tui

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lukemcguire/zombiecrawl/crawler"
	"github.com/lukemcguire/zombiecrawl/result"
)

// Model is the Bubble Tea model for the crawl TUI.
type Model struct {
	ctx             context.Context
	cancel          context.CancelFunc
	crawlerInstance *crawler.Crawler
	spinner         spinner.Model
	progressCh      <-chan crawler.CrawlEvent

	checked  int
	broken   int
	current  string
	quitting bool
	done     bool
	result   *result.Result
	err      error
	width    int
}

// NewModel creates a TUI model wired to the given crawler and progress channel.
func NewModel(ctx context.Context, cancel context.CancelFunc, crawlerInst *crawler.Crawler, progressCh <-chan crawler.CrawlEvent) Model {
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	return Model{
		ctx:             ctx,
		cancel:          cancel,
		crawlerInstance: crawlerInst,
		spinner:         spin,
		progressCh:      progressCh,
	}
}

// Init starts the spinner, crawl, and progress listener concurrently.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.startCrawl(), waitForProgress(m.progressCh))
}

// startCrawl returns a tea.Cmd that runs the crawler and sends CrawlDoneMsg.
func (m Model) startCrawl() tea.Cmd {
	return func() tea.Msg {
		res, err := m.crawlerInstance.Run(m.ctx)
		if err != nil {
			err = fmt.Errorf("crawl: %w", err)
		}
		return CrawlDoneMsg{Result: res, Err: err}
	}
}

// Update handles messages from the Bubble Tea runtime.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			m.cancel()
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width

	case CrawlProgressMsg:
		m.checked = msg.Checked
		m.broken = msg.Broken
		m.current = msg.URL
		return m, waitForProgress(m.progressCh)

	case CrawlDoneMsg:
		m.done = true
		m.result = msg.Result
		m.err = msg.Err
		return m, tea.Quit

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

// View renders the current TUI state.
func (m Model) View() string {
	if m.done && m.result != nil {
		return RenderSummary(m.result)
	}
	if m.done && m.err != nil {
		return errorStyle.Render("Error: "+m.err.Error()) + "\n"
	}
	return fmt.Sprintf("%s Crawling... checked %d, broken %d\n%s\n",
		m.spinner.View(), m.checked, m.broken,
		dimStyle.Render("  "+m.current))
}

// HasBrokenLinks reports whether the crawl found any broken links.
func (m Model) HasBrokenLinks() bool {
	return m.result != nil && len(m.result.BrokenLinks) > 0
}

// GetResult returns the crawl result for output formatting.
func (m Model) GetResult() *result.Result {
	return m.result
}
