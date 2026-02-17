# Phase 04-01: Depth Limiting for Crawl Scope Control

## Summary

Implemented depth limiting capability to restrict how deep the crawler traverses from the start URL.

## Changes

### `src/crawler/worker.go`
- Added `MaxDepth int` field to `Config` struct (default: 0 = unlimited)
- Added `Depth int` field to `CrawlJob` struct (tracks current depth, 0-indexed)
- Updated `DefaultConfig()` to initialize `MaxDepth: 0`

### `src/crawler/crawler.go`
- Start URL seeded with `Depth: 0`
- Coordinator calculates `nextDepth := crawlResult.Job.Depth + 1` for discovered links
- Depth limit check: `if !isExternal && c.cfg.MaxDepth > 0 && nextDepth > c.cfg.MaxDepth` skips enqueueing
- New jobs created with `Depth: nextDepth`

## User Decisions

| Decision | Choice |
|----------|--------|
| Depth indexing | 0-indexed (start URL = depth 0) |
| MaxDepth default | 0 = unlimited |
| Depth scope | Same-domain only; external links always validated but never crawled |

## Verification

- `go build ./...` ✓
- `go test ./src/crawler/...` ✓
- `golangci-lint run ./...` ✓

## Requirements Met

- CRWL-05: User can limit crawl depth with --depth flag
