package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lukemcguire/zombiecrawl/crawler"
	"github.com/lukemcguire/zombiecrawl/result"
)

func TestNewModel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	progressCh := make(chan crawler.CrawlEvent, 10)
	cr := crawler.New(crawler.Config{
		StartURL:       "https://example.com",
		Concurrency:    2,
		RequestTimeout: 5 * time.Second,
	}, progressCh)

	model := NewModel(ctx, cancel, cr, progressCh)

	if model.ctx != ctx {
		t.Error("expected ctx to be stored in model")
	}
	if model.cancel == nil {
		t.Error("expected cancel to be stored in model")
	}
	if model.crawlerInstance != cr {
		t.Error("expected crawler instance to be stored in model")
	}
	if model.progressCh != progressCh {
		t.Error("expected progressCh to be stored in model")
	}
	if model.checked != 0 || model.broken != 0 {
		t.Error("expected initial counters to be zero")
	}
	if model.done {
		t.Error("expected done to be false initially")
	}
}

func TestHasBrokenLinks(t *testing.T) {
	tests := []struct {
		name   string
		result *result.Result
		want   bool
	}{
		{
			name:   "nil result",
			result: nil,
			want:   false,
		},
		{
			name:   "no broken links",
			result: &result.Result{BrokenLinks: []result.LinkResult{}},
			want:   false,
		},
		{
			name: "has broken links",
			result: &result.Result{
				BrokenLinks: []result.LinkResult{
					{URL: "https://example.com/missing", StatusCode: 404},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := Model{result: tt.result}
			if got := model.HasBrokenLinks(); got != tt.want {
				t.Errorf("HasBrokenLinks() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetResult(t *testing.T) {
	tests := []struct {
		name   string
		result *result.Result
	}{
		{
			name:   "nil result",
			result: nil,
		},
		{
			name:   "empty result",
			result: &result.Result{BrokenLinks: []result.LinkResult{}},
		},
		{
			name: "result with broken links",
			result: &result.Result{
				BrokenLinks: []result.LinkResult{
					{URL: "https://example.com/missing", StatusCode: 404},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := Model{result: tt.result}
			got := model.GetResult()
			if got != tt.result {
				t.Errorf("GetResult() = %v, want %v", got, tt.result)
			}
		})
	}
}

func TestRenderSummary_NilResult(t *testing.T) {
	output := RenderSummary(nil)
	if output == "" {
		t.Error("expected non-empty output for nil result")
	}
}

func TestRenderSummary_NoBrokenLinks(t *testing.T) {
	res := &result.Result{
		BrokenLinks: []result.LinkResult{},
		Stats: result.CrawlStats{
			TotalChecked: 10,
			BrokenCount:  0,
			Duration:     2 * time.Second,
		},
	}
	output := RenderSummary(res)
	if output == "" {
		t.Error("expected non-empty output")
	}
	// The styled output should contain the core text (ANSI codes may wrap it).
	if !containsSubstring(output, "No broken links found") {
		t.Errorf("expected success message, got: %s", output)
	}
	if !containsSubstring(output, "10") {
		t.Errorf("expected URL count in output, got: %s", output)
	}
}

func TestRenderSummary_WithBrokenLinks(t *testing.T) {
	res := &result.Result{
		BrokenLinks: []result.LinkResult{
			{URL: "https://example.com/dead", StatusCode: 404, SourcePage: "https://example.com"},
			{URL: "https://example.com/err", Error: "connection refused", SourcePage: "https://example.com/about"},
		},
		Stats: result.CrawlStats{
			TotalChecked: 25,
			BrokenCount:  2,
			Duration:     3 * time.Second,
		},
	}
	output := RenderSummary(res)
	if !containsSubstring(output, "example.com/dead") {
		t.Errorf("expected broken URL in output, got: %s", output)
	}
	if !containsSubstring(output, "404") {
		t.Errorf("expected status code in output, got: %s", output)
	}
	if !containsSubstring(output, "connection refused") {
		t.Errorf("expected error message in output, got: %s", output)
	}
	if !containsSubstring(output, "2 broken links") {
		t.Errorf("expected broken count in summary, got: %s", output)
	}
}

func TestInit_ReturnsBatchCmd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	progressCh := make(chan crawler.CrawlEvent, 10)
	crawlerInst := crawler.New(crawler.Config{
		StartURL:       "https://example.com",
		Concurrency:    1,
		RequestTimeout: 5 * time.Second,
	}, progressCh)

	model := NewModel(ctx, cancel, crawlerInst, progressCh)
	cmd := model.Init()
	if cmd == nil {
		t.Error("Init() should return a non-nil batch command")
	}
}

func TestUpdate_CrawlProgressMsg(t *testing.T) {
	model := Model{
		progressCh: make(chan crawler.CrawlEvent, 10),
	}

	msg := CrawlProgressMsg{Checked: 5, Broken: 1, URL: "https://example.com/page"}
	updatedModel, cmd := model.Update(msg)
	updated := updatedModel.(Model)

	if updated.checked != 5 {
		t.Errorf("expected checked=5, got %d", updated.checked)
	}
	if updated.broken != 1 {
		t.Errorf("expected broken=1, got %d", updated.broken)
	}
	if updated.current != "https://example.com/page" {
		t.Errorf("expected current URL to be set, got %s", updated.current)
	}
	if cmd == nil {
		t.Error("expected non-nil cmd to re-subscribe to progress channel")
	}
}

func TestUpdate_CrawlDoneMsg(t *testing.T) {
	model := Model{}
	res := &result.Result{
		BrokenLinks: []result.LinkResult{{URL: "https://example.com/404", StatusCode: 404}},
		Stats:       result.CrawlStats{TotalChecked: 10, BrokenCount: 1},
	}

	updatedModel, _ := model.Update(CrawlDoneMsg{Result: res})
	updated := updatedModel.(Model)

	if !updated.done {
		t.Error("expected done=true after CrawlDoneMsg")
	}
	if updated.result != res {
		t.Error("expected result to be stored")
	}
}

func TestUpdate_SpinnerTickMsg(t *testing.T) {
	model := Model{}
	// Send a spinner tick — should not panic and should return a command.
	updatedModel, _ := model.Update(spinner.TickMsg{})
	_ = updatedModel.(Model) // should not panic
}

func TestUpdate_WindowSizeMsg(t *testing.T) {
	model := Model{}
	updatedModel, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	updated := updatedModel.(Model)

	if updated.width != 120 {
		t.Errorf("expected width=120, got %d", updated.width)
	}
}

func TestView_InProgress(t *testing.T) {
	model := Model{
		checked: 3,
		broken:  1,
		current: "https://example.com/checking",
	}
	output := model.View()
	if !strings.Contains(output, "Crawling") {
		t.Errorf("expected 'Crawling' in progress view, got: %s", output)
	}
	if !strings.Contains(output, "3") {
		t.Errorf("expected checked count in view, got: %s", output)
	}
}

func TestView_DoneWithResult(t *testing.T) {
	model := Model{
		done: true,
		result: &result.Result{
			BrokenLinks: []result.LinkResult{},
			Stats:       result.CrawlStats{TotalChecked: 5, Duration: time.Second},
		},
	}
	output := model.View()
	if !strings.Contains(output, "No broken links found") {
		t.Errorf("expected success message in done view, got: %s", output)
	}
}

func TestView_DoneWithError(t *testing.T) {
	model := Model{
		done: true,
		err:  context.Canceled,
	}
	output := model.View()
	if !strings.Contains(output, "Error") {
		t.Errorf("expected error message in done view, got: %s", output)
	}
}

// containsSubstring checks for a substring in a string that may contain ANSI codes.
func containsSubstring(haystack, needle string) bool {
	return len(haystack) > 0 && len(needle) > 0 &&
		strings.Contains(haystack, needle)
}
