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
	concurrency := flag.Int("c", 17, "number of concurrent workers")
	flag.IntVar(concurrency, "concurrency", 17, "number of concurrent workers")

	rateLimit := flag.Int("r", 10, "requests per second (default 10)")
	flag.IntVar(rateLimit, "rate-limit", 10, "requests per second")

	retries := flag.Int("n", 2, "number of retries for transient errors (default 2 = 3 attempts)")
	flag.IntVar(retries, "retries", 2, "number of retries for transient errors")

	retryDelay := flag.Duration("retry-delay", 1*time.Second, "base delay between retries (default 1s)")

	userAgent := flag.String("U", "zombiecrawl/1.0 (+https://github.com/lukemcguire/zombiecrawl)", "user agent string")
	flag.StringVar(userAgent, "user-agent", "zombiecrawl/1.0 (+https://github.com/lukemcguire/zombiecrawl)", "user agent string")

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
