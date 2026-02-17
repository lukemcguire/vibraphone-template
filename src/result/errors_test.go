package result

import (
	"context"
	"net"
	"testing"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		statusCode     int
		isRedirectLoop bool
		want           ErrorCategory
	}{
		{
			name:           "redirect loop",
			err:            nil,
			statusCode:     0,
			isRedirectLoop: true,
			want:           CategoryRedirectLoop,
		},
		{
			name:           "4xx status",
			err:            nil,
			statusCode:     404,
			isRedirectLoop: false,
			want:           Category4xx,
		},
		{
			name:           "5xx status",
			err:            nil,
			statusCode:     500,
			isRedirectLoop: false,
			want:           Category5xx,
		},
		{
			name:           "timeout error",
			err:            context.DeadlineExceeded,
			statusCode:     0,
			isRedirectLoop: false,
			want:           CategoryTimeout,
		},
		{
			name:           "no error no status",
			err:            nil,
			statusCode:     0,
			isRedirectLoop: false,
			want:           CategoryUnknown,
		},
		{
			name:           "3xx status is unknown",
			err:            nil,
			statusCode:     301,
			isRedirectLoop: false,
			want:           CategoryUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyError(tt.err, tt.statusCode, tt.isRedirectLoop)
			if got != tt.want {
				t.Errorf("ClassifyError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClassifyError_DNSFailure(t *testing.T) {
	// Create a DNS error
	dnsErr := &net.DNSError{
		Err:  "no such host",
		Name: "example.invalid",
	}

	got := ClassifyError(dnsErr, 0, false)
	if got != CategoryDNSFailure {
		t.Errorf("ClassifyError(DNSError) = %v, want %v", got, CategoryDNSFailure)
	}
}

func TestFormatCategory(t *testing.T) {
	tests := []struct {
		cat  ErrorCategory
		want string
	}{
		{CategoryTimeout, "Timeouts"},
		{CategoryDNSFailure, "DNS Failures"},
		{CategoryConnectionRefused, "Connection Refused"},
		{Category4xx, "Client Errors (4xx)"},
		{Category5xx, "Server Errors (5xx)"},
		{CategoryRedirectLoop, "Redirect Loops"},
		{CategoryUnknown, "Other Errors"},
	}

	for _, tt := range tests {
		t.Run(string(tt.cat), func(t *testing.T) {
			got := FormatCategory(tt.cat)
			if got != tt.want {
				t.Errorf("FormatCategory(%v) = %v, want %v", tt.cat, got, tt.want)
			}
		})
	}
}
