package result

import (
	"context"
	"errors"
	"net"
	"strings"
)

// ErrorCategory represents the classification of a crawl error.
type ErrorCategory string

const (
	CategoryTimeout           ErrorCategory = "timeout"
	CategoryDNSFailure        ErrorCategory = "dns_failure"
	CategoryConnectionRefused ErrorCategory = "connection_refused"
	Category4xx               ErrorCategory = "4xx"
	Category5xx               ErrorCategory = "5xx"
	CategoryRedirectLoop      ErrorCategory = "redirect_loop"
	CategoryUnknown           ErrorCategory = "unknown"
)

// ClassifyError determines the error category based on the error, HTTP status code,
// and whether a redirect loop was detected.
func ClassifyError(err error, statusCode int, isRedirectLoop bool) ErrorCategory {
	// Check redirect loop first (highest priority)
	if isRedirectLoop {
		return CategoryRedirectLoop
	}

	// Check HTTP status codes
	if statusCode > 0 {
		if statusCode >= 400 && statusCode <= 499 {
			return Category4xx
		}
		if statusCode >= 500 {
			return Category5xx
		}
	}

	// If no error, return unknown
	if err == nil {
		return CategoryUnknown
	}

	// Check for timeout
	if errors.Is(err, context.DeadlineExceeded) {
		return CategoryTimeout
	}

	// Check for DNS failure
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return CategoryDNSFailure
	}

	// Check for connection refused or other net operation errors
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Op == "dial" && strings.Contains(opErr.Error(), "connection refused") {
			return CategoryConnectionRefused
		}
		// Check if it's a timeout via OpError
		if opErr.Timeout() {
			return CategoryTimeout
		}
	}

	// Fallback to unknown
	return CategoryUnknown
}

// FormatCategory returns a human-readable label for an error category.
func FormatCategory(cat ErrorCategory) string {
	switch cat {
	case CategoryTimeout:
		return "Timeouts"
	case CategoryDNSFailure:
		return "DNS Failures"
	case CategoryConnectionRefused:
		return "Connection Refused"
	case Category4xx:
		return "Client Errors (4xx)"
	case Category5xx:
		return "Server Errors (5xx)"
	case CategoryRedirectLoop:
		return "Redirect Loops"
	default:
		return "Other Errors"
	}
}
