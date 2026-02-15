# Phase 2: Bubble Tea TUI - Research

**Researched:** 2026-02-15
**Domain:** Charm ecosystem TUI (Bubble Tea + Lip Gloss + Bubbles)
**Confidence:** HIGH

## Summary

Phase 2 replaces the current `fmt.Printf` output in the crawler with a real-time Bubble Tea TUI showing live progress (spinner, counters) during crawl and a Lip Gloss-styled summary table after completion. The core integration challenge is that `crawler.Run()` currently writes directly to stdout via `fmt.Printf` on lines 101-106 of `crawler.go`. This must be refactored to emit events/messages that the TUI model can consume.

The Charm ecosystem is mature and well-documented. Bubble Tea v1 (v1.3.10, Sep 2025) is the correct choice -- v2 is still in release candidate stage (v2.0.0-rc.2, Nov 2025) and not yet stable. The standard pattern is: run the crawler in a `tea.Cmd` goroutine, send progress messages back to the TUI model via channel or `Program.Send()`, and render a final styled summary with Lip Gloss tables.

**Primary recommendation:** Use Bubble Tea v1.3.x with Bubbles spinner component and Lip Gloss table sub-package. Refactor crawler to accept a callback/channel for progress events instead of printing to stdout.

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| github.com/charmbracelet/bubbletea | v1.3.x | TUI framework (Elm architecture) | De facto Go TUI framework, stable v1 |
| github.com/charmbracelet/lipgloss | v1.1.x | Terminal styling and table rendering | Charm companion for layout/color |
| github.com/charmbracelet/bubbles | v1.0.x | Pre-built TUI components (spinner) | Official component library for Bubble Tea |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| lipgloss/table | (sub-package) | Summary table rendering | Post-crawl broken links table |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Bubble Tea v1 | Bubble Tea v2 (RC) | v2 has better renderer but is not stable; v1 is production-ready |
| Bubbles spinner | Custom spinner | No reason to hand-roll; bubbles has 12+ spinner styles |
| lipgloss/table | Custom table formatting | lipgloss/table handles borders, column width, styling out of the box |

**Installation:**
```bash
cd src && go get github.com/charmbracelet/bubbletea@latest github.com/charmbracelet/lipgloss@latest github.com/charmbracelet/bubbles@latest
```

## Architecture Patterns

### Recommended Project Structure
```
src/
├── main.go              # Parse flags, create TUI program, run, handle exit code
├── tui/
│   ├── model.go         # Bubble Tea model (Init, Update, View)
│   ├── messages.go      # Custom message types (CrawlProgressMsg, CrawlDoneMsg)
│   └── styles.go        # Lip Gloss style definitions
├── crawler/
│   ├── crawler.go       # Refactored: accepts event callback, no stdout writes
│   ├── worker.go        # Unchanged
│   ├── extract.go       # Unchanged
│   └── events.go        # CrawlEvent type for progress reporting
├── result/
│   ├── result.go        # Unchanged
│   └── printer.go       # Keep for non-TUI fallback or remove
└── urlutil/             # Unchanged
```

### Pattern 1: Elm Architecture (Model-Update-View)
**What:** Bubble Tea uses The Elm Architecture -- a unidirectional data flow where the Model holds state, Update handles messages and returns new state + commands, and View renders the model as a string.
**When to use:** Always -- this is how Bubble Tea works.
**Example:**
```go
// Source: pkg.go.dev/github.com/charmbracelet/bubbletea
type model struct {
    spinner  spinner.Model
    checked  int
    broken   int
    current  string
    done     bool
    result   *result.Result
    quitting bool
}

func (m model) Init() tea.Cmd {
    return tea.Batch(m.spinner.Tick, m.startCrawl())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        if msg.String() == "ctrl+c" {
            m.quitting = true
            return m, tea.Quit
        }
    case CrawlProgressMsg:
        m.checked = msg.Checked
        m.broken = msg.Broken
        m.current = msg.URL
        return m, nil
    case CrawlDoneMsg:
        m.done = true
        m.result = msg.Result
        return m, tea.Quit
    case spinner.TickMsg:
        var cmd tea.Cmd
        m.spinner, cmd = m.spinner.Update(msg)
        return m, cmd
    }
    return m, nil
}

func (m model) View() string {
    if m.done {
        return renderSummary(m.result)
    }
    return fmt.Sprintf("%s Crawling... checked %d, broken %d\n  %s",
        m.spinner.View(), m.checked, m.broken, m.current)
}
```

### Pattern 2: Async Crawler Integration via tea.Cmd
**What:** Run the crawler in a tea.Cmd (goroutine). The crawler sends progress events through a channel. A listener command converts channel events to Bubble Tea messages.
**When to use:** Integrating long-running async work with the TUI.
**Example:**
```go
// Source: github.com/charmbracelet/bubbletea/issues/25, pkg.go.dev docs

// Message types
type CrawlProgressMsg struct {
    Checked int
    Broken  int
    URL     string
}

type CrawlDoneMsg struct {
    Result *result.Result
    Err    error
}

// Start crawl returns a command that launches the crawler
func (m model) startCrawl() tea.Cmd {
    return func() tea.Msg {
        // This runs in its own goroutine
        res, err := m.crawler.Run(m.ctx)
        return CrawlDoneMsg{Result: res, Err: err}
    }
}

// Alternative: channel-based progress with waitForProgress pattern
func waitForProgress(ch <-chan CrawlProgressMsg) tea.Cmd {
    return func() tea.Msg {
        msg, ok := <-ch
        if !ok {
            return nil // channel closed
        }
        return msg
    }
}

// In Update, re-subscribe after each progress message:
case CrawlProgressMsg:
    m.checked = msg.Checked
    m.broken = msg.Broken
    return m, waitForProgress(m.progressCh)
```

### Pattern 3: Lip Gloss Summary Table
**What:** After crawl completes, render a styled table of broken links using lipgloss/table.
**When to use:** Final output display.
**Example:**
```go
// Source: pkg.go.dev/github.com/charmbracelet/lipgloss/table
import (
    "github.com/charmbracelet/lipgloss"
    "github.com/charmbracelet/lipgloss/table"
)

func renderSummary(res *result.Result) string {
    if len(res.BrokenLinks) == 0 {
        okStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
        return okStyle.Render("No broken links found!")
    }

    rows := make([][]string, len(res.BrokenLinks))
    for i, link := range res.BrokenLinks {
        status := link.Error
        if status == "" {
            status = fmt.Sprintf("%d", link.StatusCode)
        }
        rows[i] = []string{link.URL, status, link.SourcePage}
    }

    t := table.New().
        Headers("URL", "Status", "Found On").
        Rows(rows...).
        Border(lipgloss.RoundedBorder()).
        StyleFunc(func(row, col int) lipgloss.Style {
            if row == table.HeaderRow {
                return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
            }
            return lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
        })

    return t.String()
}
```

### Pattern 4: Context Cancellation for Ctrl+C
**What:** Use `tea.WithContext(ctx)` to wire signal handling. When user presses Ctrl+C, the context is cancelled, which propagates to the crawler.
**When to use:** Graceful shutdown.
**Example:**
```go
// In main.go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

p := tea.NewProgram(
    newModel(ctx, cfg),
    tea.WithContext(ctx),
)

m, err := p.Run()
// After Run returns, check final model for exit code
finalModel := m.(model)
if finalModel.result != nil && len(finalModel.result.BrokenLinks) > 0 {
    os.Exit(1)
}
```

### Anti-Patterns to Avoid
- **Writing to stdout from crawler goroutines:** Bubble Tea owns stdout. Any fmt.Printf from the crawler will corrupt the TUI. All output must go through the model's View().
- **Mutating model outside Update:** The Elm architecture requires all state changes through messages. Never modify model fields from a goroutine.
- **Blocking in Update:** Never do I/O or long operations in Update. Return a tea.Cmd instead.
- **Using tea.Println for live data:** tea.Println prints above the TUI permanently -- use it for logging, not for live-updating counters.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Spinner animation | Custom frame ticker | bubbles/spinner | 12+ styles, handles tick timing, composable |
| Table formatting | fmt.Sprintf alignment | lipgloss/table | Borders, column width, header styling, Unicode support |
| Terminal styling | ANSI escape codes | lipgloss | Color profile detection, adaptive colors, composable styles |
| TUI event loop | Custom goroutine + raw terminal | bubbletea | Input handling, resize, alt screen, renderer all handled |

**Key insight:** The Charm ecosystem is designed to work together. Using individual pieces (e.g., only lipgloss for tables) without Bubble Tea is possible but the real value is the integrated Elm architecture that makes async TUI updates safe and predictable.

## Common Pitfalls

### Pitfall 1: Crawler Still Writes to Stdout
**What goes wrong:** The current crawler.go has `fmt.Printf` calls on lines 101-106. If these remain, they corrupt the Bubble Tea TUI rendering.
**Why it happens:** Forgetting to refactor the crawler when adding the TUI layer.
**How to avoid:** Refactor crawler.Run() to accept a progress callback `func(CrawlEvent)` or a channel. Remove all fmt.Printf calls from crawler.go.
**Warning signs:** Garbled terminal output, flickering, characters appearing in wrong places.

### Pitfall 2: Forgetting to Re-subscribe to Channel
**What goes wrong:** Using a channel-based pattern but only calling `waitForProgress` once. After the first message, no more progress updates arrive.
**Why it happens:** tea.Cmd is a one-shot function. Each channel read needs a new command.
**How to avoid:** In the Update handler for CrawlProgressMsg, always return `waitForProgress(m.ch)` as the next command.
**Warning signs:** TUI shows first progress update then freezes.

### Pitfall 3: Not Handling Ctrl+C Properly
**What goes wrong:** Ctrl+C kills the TUI but the crawler goroutine keeps running, or the terminal is left in a bad state.
**Why it happens:** Not propagating context cancellation to the crawler.
**How to avoid:** Wire context through `tea.WithContext(ctx)` and ensure the crawler respects `ctx.Done()`. The crawler already checks `ctx.Done()` in its worker loop (line 74 of crawler.go).
**Warning signs:** Process hangs after Ctrl+C, terminal needs `reset`.

### Pitfall 4: Exit Code After tea.Program.Run()
**What goes wrong:** Calling `os.Exit()` inside a tea.Cmd or Update, which bypasses Bubble Tea's cleanup.
**Why it happens:** Wanting to set exit code 1 for broken links.
**How to avoid:** Let `p.Run()` return naturally (via `tea.Quit`). Inspect the final model afterwards to determine exit code. Call `os.Exit()` only in main() after Run() returns.
**Warning signs:** Terminal left in raw mode, alt screen not cleared.

### Pitfall 5: Alt Screen vs Inline Mode
**What goes wrong:** Using `tea.WithAltScreen()` clears all output when the program exits, losing the summary.
**Why it happens:** Alt screen is the "full window" mode that restores the previous terminal content on exit.
**How to avoid:** Do NOT use `WithAltScreen()` for this tool. Use inline mode (the default) so the final summary table remains visible in the terminal after exit. The spinner/progress will be overwritten by the summary, which is the desired behavior.
**Warning signs:** Summary table disappears when program exits.

## Code Examples

### Complete Model Skeleton
```go
// Source: pkg.go.dev/github.com/charmbracelet/bubbletea, verified patterns
package tui

import (
    "context"
    "fmt"

    "github.com/charmbracelet/bubbles/spinner"
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
    "github.com/charmbracelet/lipgloss/table"

    "github.com/lukemcguire/zombiecrawl/crawler"
    "github.com/lukemcguire/zombiecrawl/result"
)

type model struct {
    ctx       context.Context
    cancel    context.CancelFunc
    crawler   *crawler.Crawler
    spinner   spinner.Model
    progressCh <-chan crawler.CrawlEvent

    checked  int
    broken   int
    current  string
    done     bool
    result   *result.Result
    err      error
}

func NewModel(ctx context.Context, cancel context.CancelFunc, c *crawler.Crawler, ch <-chan crawler.CrawlEvent) model {
    s := spinner.New()
    s.Spinner = spinner.Dot
    s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

    return model{
        ctx:        ctx,
        cancel:     cancel,
        crawler:    c,
        spinner:    s,
        progressCh: ch,
    }
}
```

### Spinner Setup
```go
// Source: pkg.go.dev/github.com/charmbracelet/bubbles/spinner
s := spinner.New()
s.Spinner = spinner.Dot  // Or: spinner.Line, spinner.MiniDot, spinner.Pulse
s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

// In Init(), start the spinner tick:
func (m model) Init() tea.Cmd {
    return tea.Batch(m.spinner.Tick, m.startCrawl(), waitForProgress(m.progressCh))
}

// In Update(), forward tick messages to spinner:
case spinner.TickMsg:
    var cmd tea.Cmd
    m.spinner, cmd = m.spinner.Update(msg)
    return m, cmd
```

### Main Function Integration
```go
// Source: verified pattern from Bubble Tea docs
func main() {
    // Parse flags (keep existing flag logic)
    // ...

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Handle OS signals
    go func() {
        sigCh := make(chan os.Signal, 1)
        signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
        <-sigCh
        cancel()
    }()

    cfg := crawler.Config{...}
    progressCh := make(chan crawler.CrawlEvent, 100)
    c := crawler.New(cfg, progressCh) // Refactored constructor

    m := tui.NewModel(ctx, cancel, c, progressCh)
    p := tea.NewProgram(m) // No WithAltScreen -- inline mode

    finalModel, err := p.Run()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }

    fm := finalModel.(tui.Model) // Type assert to get result
    if fm.HasBrokenLinks() {
        os.Exit(1)
    }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Bubble Tea v0.x | Bubble Tea v1.3.x (stable) | 2024 | Stable API, use v1 |
| lipgloss v0.x | lipgloss v1.1.x (stable) | 2025 | Table sub-package now included |
| Custom table rendering | lipgloss/table sub-package | lipgloss v0.10+ | No need for separate table library |
| p.Start() | p.Run() | Bubble Tea v0.24+ | Start() is deprecated, use Run() |

**Deprecated/outdated:**
- `tea.Program.Start()`: Deprecated. Use `Run()` instead.
- `tea.Program.StartReturningModel()`: Deprecated. `Run()` returns the model.
- Bubble Tea v2: Still RC (v2.0.0-rc.2). Do not use for production. Stick with v1.3.x.
- Lipgloss v2: Still beta. Stick with v1.1.x.

## Open Questions

1. **Crawler refactoring approach: callback vs channel?**
   - What we know: Both patterns work. Channel with `waitForProgress` cmd is the idiomatic Bubble Tea pattern. Callback is simpler but requires the callback to be goroutine-safe.
   - Recommendation: Use a buffered channel. The crawler sends `CrawlEvent` structs to the channel, the TUI reads them via `waitForProgress` commands. This keeps the crawler decoupled from Bubble Tea.

2. **Should the existing `result.PrintResults` be kept?**
   - What we know: The current text printer in `printer.go` works for non-TUI output.
   - Recommendation: Keep it but it will no longer be called from main. It serves as a fallback and is useful for testing. The TUI will use its own lipgloss-based renderer.

3. **Terminal width for table rendering**
   - What we know: Lip Gloss table supports `.Width(n)` for constraining table width. Bubble Tea provides `tea.WindowSizeMsg` on resize.
   - Recommendation: Capture terminal width from `tea.WindowSizeMsg` and pass to table renderer for responsive output.

## Sources

### Primary (HIGH confidence)
- [pkg.go.dev/github.com/charmbracelet/bubbletea](https://pkg.go.dev/github.com/charmbracelet/bubbletea) - v1.3.10 API, Program options, Cmd patterns
- [pkg.go.dev/github.com/charmbracelet/bubbles/spinner](https://pkg.go.dev/github.com/charmbracelet/bubbles/spinner) - Spinner types and embedding pattern
- [pkg.go.dev/github.com/charmbracelet/lipgloss/table](https://pkg.go.dev/github.com/charmbracelet/lipgloss/table) - Table API, styling, borders
- [pkg.go.dev/github.com/charmbracelet/bubbletea/v2](https://pkg.go.dev/github.com/charmbracelet/bubbletea/v2) - v2 status (beta, not stable)

### Secondary (MEDIUM confidence)
- [Bubble Tea v2 Discussion #1374](https://github.com/charmbracelet/bubbletea/discussions/1374) - v2 migration info, confirmed RC status
- [Bubble Tea Issue #25](https://github.com/charmbracelet/bubbletea/issues/25) - Sending messages from outside program loop
- [GitHub charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss) - Color support, layout utilities

### Tertiary (LOW confidence)
- None -- all findings verified with primary or secondary sources.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - Verified versions and APIs via pkg.go.dev
- Architecture: HIGH - Elm architecture is well-documented, patterns verified from official docs and examples
- Pitfalls: HIGH - Derived from direct code inspection (crawler.go stdout writes) and documented Bubble Tea behaviors
- Integration: MEDIUM - Channel-based async pattern is well-established but specific crawler refactoring approach needs validation during implementation

**Research date:** 2026-02-15
**Valid until:** 2026-04-15 (stable ecosystem, v1 is mature)
