package crawler

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/lukemcguire/zombiecrawl/result"
)

// RetryPolicy configures retry behavior for failed requests.
type RetryPolicy struct {
	MaxRetries int           // Maximum number of retries (2 = 3 total attempts)
	BaseDelay  time.Duration // Initial backoff delay (1s)
	MaxDelay   time.Duration // Maximum backoff cap (30s)
}

// DefaultRetryPolicy returns a RetryPolicy with sensible defaults:
// 2 retries (3 attempts), 1s base delay, 30s max delay.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries: 2,
		BaseDelay:  1 * time.Second,
		MaxDelay:   30 * time.Second,
	}
}

// CheckURLWithRetry wraps CheckURL with exponential backoff retry logic.
// It retries on transient failures (network errors, 5xx, 429) but not on
// permanent failures (4xx except 429).
func CheckURLWithRetry(ctx context.Context, client *http.Client, job CrawlJob, cfg Config, policy RetryPolicy) CrawlResult {
	backoff := policy.BaseDelay
	var lastResult CrawlResult
	var attempts int

	for attempt := 0; attempt <= policy.MaxRetries; attempt++ {
		attempts = attempt + 1

		// Wait with backoff before retry (not on first attempt)
		if attempt > 0 {
			select {
			case <-ctx.Done():
				// Context cancelled during wait
				lastResult.Job = job
				if lastResult.Result == nil {
					lastResult.Result = &result.LinkResult{
						URL:           job.URL,
						SourcePage:    job.SourcePage,
						IsExternal:    job.IsExternal,
						Error:         ctx.Err().Error(),
						ErrorCategory: result.CategoryUnknown,
					}
				}
				return lastResult
			case <-time.After(backoff):
				// Double backoff for next retry
				backoff = min(backoff*2, policy.MaxDelay)
			}
		}

		// Attempt the request
		lastResult = CheckURL(ctx, client, job, cfg)

		// Success: no error and status < 400
		if lastResult.Result == nil && lastResult.Err == nil {
			return lastResult
		}

		// Check if we should retry
		if !shouldRetry(lastResult) {
			return lastResult
		}
	}

	// All retries exhausted - append retry info to error message
	if lastResult.Result != nil && lastResult.Result.Error != "" {
		lastResult.Result.Error = fmt.Sprintf("%s (after %d attempts)", lastResult.Result.Error, attempts)
	}

	return lastResult
}

// shouldRetry determines if a failed request should be retried.
// Returns true for:
// - Network errors (timeout, connection refused, DNS failure)
// - HTTP 429 (rate limited)
// - HTTP 5xx (server errors)
// Returns false for:
// - HTTP 4xx except 429 (client errors)
func shouldRetry(res CrawlResult) bool {
	// Check for network errors (status code 0)
	if res.Result != nil && res.Result.StatusCode == 0 {
		return isRetryableNetworkError(res.Result.Error)
	}

	// Check status codes
	status := 0
	if res.Result != nil {
		status = res.Result.StatusCode
	}

	// 429 Too Many Requests - retry
	if status == 429 {
		return true
	}

	// 5xx server errors - retry
	if status >= 500 {
		return true
	}

	// 4xx client errors (except 429) - don't retry
	if status >= 400 {
		return false
	}

	// Check CrawlResult.Err for network-level errors
	if res.Err != nil {
		return isRetryableError(res.Err)
	}

	return false
}

// isRetryableNetworkError determines if an error string indicates a retryable condition.
func isRetryableNetworkError(errMsg string) bool {
	// These error patterns indicate transient issues
	retryablePatterns := []string{
		"timeout",
		"deadline exceeded",
		"connection refused",
		"connection reset",
		"no such host",
		"DNS",
		"temporary failure",
	}

	for _, pattern := range retryablePatterns {
		if containsIgnoreCase(errMsg, pattern) {
			return true
		}
	}
	return false
}

// isRetryableError checks if an error type is retryable.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Context deadline exceeded
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Network operation errors (covers timeout, connection refused)
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	// DNS errors
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr)
}

// containsIgnoreCase checks if s contains substr (case-insensitive).
func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) && containsAt(s, substr, 0)) ||
		containsIgnoreCase(s[1:], substr))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			sc := s[i+j]
			subc := substr[j]
			if sc >= 'A' && sc <= 'Z' {
				sc += 32
			}
			if subc >= 'A' && subc <= 'Z' {
				subc += 32
			}
			if sc != subc {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
