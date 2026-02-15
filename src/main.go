package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lukemcguire/zombiecrawl/crawler"
	"github.com/lukemcguire/zombiecrawl/result"
)

func main() {
	concurrency := flag.Int("c", 17, "number of concurrent workers")
	flag.IntVar(concurrency, "concurrency", 17, "number of concurrent workers")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: zombiecrawl [flags] <url>")
		fmt.Fprintln(os.Stderr, "Flags:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	rawURL := flag.Arg(0)
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		fmt.Fprintf(os.Stderr, "Invalid URL: %s\nURL must start with http:// or https://\n", rawURL)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := crawler.Config{
		StartURL:       rawURL,
		Concurrency:    *concurrency,
		RequestTimeout: 10 * time.Second,
	}

	c := crawler.New(cfg, nil)

	results, err := c.Run(ctx)
	if err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if ctx.Err() != nil {
		fmt.Fprintln(os.Stderr, "\nCrawl interrupted. Showing partial results...")
	}

	result.PrintResults(os.Stdout, results)

	if len(results.BrokenLinks) > 0 {
		os.Exit(1)
	}
}
