// Package main provides the zombiecrawl CLI entrypoint.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lukemcguire/zombiecrawl/crawler"
	"github.com/lukemcguire/zombiecrawl/result"
	"github.com/lukemcguire/zombiecrawl/tui"
)

// cliFlags holds parsed command-line flags.
type cliFlags struct {
	concurrency int
	rateLimit   int
	retries     int
	retryDelay  time.Duration
	userAgent   string
	depth       int
	outputJSON  bool
	outputCSV   bool
	outputFile  string
}

// parseFlags parses command-line flags and returns the parsed values.
func parseFlags() *cliFlags {
	opts := &cliFlags{}
	flag.IntVar(&opts.concurrency, "concurrency", 10, "number of concurrent workers")
	flag.IntVar(&opts.rateLimit, "rate-limit", 10, "requests per second")
	flag.IntVar(&opts.retries, "retries", 2, "number of retries for transient errors")
	flag.DurationVar(&opts.retryDelay, "retry-delay", time.Second, "base delay between retries")
	flag.StringVar(&opts.userAgent, "user-agent", "zombiecrawl/1.0 (+https://github.com/lukemcguire/zombiecrawl)", "user agent string")

	// Depth control
	flag.IntVar(&opts.depth, "d", 0, "maximum crawl depth (0 = unlimited)")
	flag.IntVar(&opts.depth, "depth", 0, "maximum crawl depth (0 = unlimited)")

	// Output format
	flag.BoolVar(&opts.outputJSON, "j", false, "output results as JSON")
	flag.BoolVar(&opts.outputJSON, "json", false, "output results as JSON")
	flag.BoolVar(&opts.outputCSV, "c", false, "output results as CSV")
	flag.BoolVar(&opts.outputCSV, "csv", false, "output results as CSV")
	flag.StringVar(&opts.outputFile, "o", "", "write JSON/CSV output to file")
	flag.StringVar(&opts.outputFile, "output", "", "write JSON/CSV output to file")

	flag.Parse()
	return opts
}

// validateFlags validates flag combinations and returns an error if invalid.
func validateFlags(opts *cliFlags) error {
	if opts.outputJSON && opts.outputCSV {
		return fmt.Errorf("--json and --csv are mutually exclusive")
	}
	return nil
}

// buildCrawlerConfig creates a crawler.Config from flags and the target URL.
func buildCrawlerConfig(opts *cliFlags, rawURL string) crawler.Config {
	return crawler.Config{
		StartURL:       rawURL,
		Concurrency:    opts.concurrency,
		RequestTimeout: 10 * time.Second,
		RateLimit:      opts.rateLimit,
		UserAgent:      opts.userAgent,
		MaxDepth:       opts.depth,
		RetryPolicy: crawler.RetryPolicy{
			MaxRetries: opts.retries,
			BaseDelay:  opts.retryDelay,
			MaxDelay:   30 * time.Second,
		},
	}
}

// runTUI creates and runs the TUI, returning the final model.
func runTUI(ctx context.Context, cancel context.CancelFunc, cfg crawler.Config) (tui.Model, error) {
	progressCh := make(chan crawler.CrawlEvent, 100)
	crawlerInstance, err := crawler.New(cfg, progressCh)
	if err != nil {
		return tui.Model{}, fmt.Errorf("create crawler: %w", err)
	}

	tuiModel := tui.NewModel(ctx, cancel, crawlerInstance, progressCh)
	program := tea.NewProgram(tuiModel)

	finalModel, err := program.Run()
	if err != nil {
		return tui.Model{}, fmt.Errorf("run tui: %w", err)
	}

	return finalModel.(tui.Model), nil
}

// writeResults writes structured output to the specified writer.
func writeResults(writer io.Writer, links []result.LinkResult, useJSON bool) error {
	if useJSON {
		if err := result.WriteJSON(writer, links); err != nil {
			return fmt.Errorf("write json: %w", err)
		}
		return nil
	}
	if err := result.WriteCSV(writer, links); err != nil {
		return fmt.Errorf("write csv: %w", err)
	}
	return nil
}

// writeStructuredOutput handles writing JSON/CSV output to stdout or a file.
func writeStructuredOutput(opts *cliFlags, model tui.Model) error {
	crawlResult := model.GetResult()
	if crawlResult == nil {
		return nil
	}

	var writer io.Writer = os.Stdout
	if opts.outputFile != "" {
		outFile, err := os.Create(opts.outputFile)
		if err != nil {
			return fmt.Errorf("create output file: %w", err)
		}
		defer func() {
			if cerr := outFile.Close(); cerr != nil {
				fmt.Fprintf(os.Stderr, "Error closing output file: %v\n", cerr)
			}
		}()
		writer = outFile
	}

	// Default to JSON if -o specified without format
	useJSON := opts.outputJSON || (!opts.outputCSV && opts.outputFile != "")

	return writeResults(writer, crawlResult.BrokenLinks, useJSON)
}

func main() {
	opts := parseFlags()

	if err := validateFlags(opts); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: zombiecrawl [flags] <url>")
		fmt.Fprintln(os.Stderr, "Flags:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	rawURL := flag.Arg(0)
	parsedURL, err := url.Parse(rawURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		fmt.Fprintf(os.Stderr, "Invalid URL: %s\nURL must start with http:// or https://\n", rawURL)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := buildCrawlerConfig(opts, rawURL)

	finalTUIModel, err := runTUI(ctx, cancel, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Write structured output if requested
	if opts.outputJSON || opts.outputCSV || opts.outputFile != "" {
		if err := writeStructuredOutput(opts, finalTUIModel); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	if finalTUIModel.HasBrokenLinks() {
		os.Exit(1)
	}
}
