// Package main provides the zombiecrawl CLI entrypoint.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lukemcguire/zombiecrawl/crawler"
	"github.com/lukemcguire/zombiecrawl/tui"
)

func main() {
	concurrency := flag.Int("concurrency", 10, "number of concurrent workers")
	rateLimit := flag.Int("rate-limit", 10, "requests per second")
	retries := flag.Int("retries", 2, "number of retries for transient errors")
	retryDelay := flag.Duration("retry-delay", time.Second, "base delay between retries")
	userAgent := flag.String("user-agent", "zombiecrawl/1.0 (+https://github.com/lukemcguire/zombiecrawl)", "user agent string")

	flag.Parse()

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

	cfg := crawler.Config{
		StartURL:       rawURL,
		Concurrency:    *concurrency,
		RequestTimeout: 10 * time.Second,
		RateLimit:      *rateLimit,
		UserAgent:      *userAgent,
		RetryPolicy: crawler.RetryPolicy{
			MaxRetries: *retries,
			BaseDelay:  *retryDelay,
			MaxDelay:   30 * time.Second,
		},
	}

	progressCh := make(chan crawler.CrawlEvent, 100)
	crawlerInstance := crawler.New(cfg, progressCh)

	tuiModel := tui.NewModel(ctx, cancel, crawlerInstance, progressCh)
	program := tea.NewProgram(tuiModel)

	finalModel, err := program.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	finalTUIModel := finalModel.(tui.Model)
	if finalTUIModel.HasBrokenLinks() {
		os.Exit(1)
	}
}
