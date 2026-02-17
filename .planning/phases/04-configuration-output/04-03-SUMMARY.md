# 04-03: CLI Flag Integration

## Summary

Integrated depth control and structured output flags into the CLI entry point, exposing Phase 4 features to users via command-line flags with proper validation and output handling.

## Changes

### CLI Flags Added

| Flag | Short | Description |
|------|-------|-------------|
| `--depth` | `-d` | Maximum crawl depth (0 = unlimited) |
| `--json` | `-j` | Output results as JSON |
| `--csv` | `-c` | Output results as CSV |
| `--output` | `-o` | Write JSON/CSV output to file |

### Validation

- `--json` and `--csv` are mutually exclusive - error and exit if both specified
- If `-o` is specified without `--json` or `--csv`, defaults to JSON format

### Output Behavior

- TUI always displays regardless of output format flags
- File output is written after crawl completes
- Files are silently overwritten (uses `os.Create`)

## Files Modified

- `src/main.go` - CLI flag definitions, validation, and output integration

## Verification

- `go build ./...` compiles without errors
- `go test ./...` passes
- All success criteria met

## Truths Delivered

- User can run `zombiecrawl --depth 1 <url>` to limit crawl depth
- User can run `zombiecrawl --json <url>` to get JSON output
- User can run `zombiecrawl --csv <url>` to get CSV output
- User can run `zombiecrawl --json -o results.json <url>` to save to file
- Specifying both `--json` and `--csv` shows error and exits
- TUI always displays regardless of output format flags
- File output is written after crawl completes
