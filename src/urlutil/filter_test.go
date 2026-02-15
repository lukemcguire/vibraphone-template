package urlutil

import "testing"

func TestIsSameDomain(t *testing.T) {
	tests := []struct {
		name      string
		targetURL string
		baseHost  string
		expected  bool
	}{
		{
			name:      "same host",
			targetURL: "https://example.com/page",
			baseHost:  "example.com",
			expected:  true,
		},
		{
			name:      "subdomain match",
			targetURL: "https://blog.example.com/post",
			baseHost:  "example.com",
			expected:  true,
		},
		{
			name:      "deep subdomain",
			targetURL: "https://a.b.example.com/",
			baseHost:  "example.com",
			expected:  true,
		},
		{
			name:      "different domain",
			targetURL: "https://other.com/page",
			baseHost:  "example.com",
			expected:  false,
		},
		{
			name:      "different TLD",
			targetURL: "https://example.org/",
			baseHost:  "example.com",
			expected:  false,
		},
		{
			name:      "scheme agnostic",
			targetURL: "http://example.com/page",
			baseHost:  "example.com",
			expected:  true,
		},
		{
			name:      "partial suffix mismatch",
			targetURL: "https://notexample.com",
			baseHost:  "example.com",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSameDomain(tt.targetURL, tt.baseHost)
			if got != tt.expected {
				t.Errorf("IsSameDomain(%q, %q) = %v, want %v", tt.targetURL, tt.baseHost, got, tt.expected)
			}
		})
	}
}

func TestIsHTTPScheme(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "https scheme",
			input:    "https://example.com",
			expected: true,
		},
		{
			name:     "http scheme",
			input:    "http://example.com",
			expected: true,
		},
		{
			name:     "mailto scheme",
			input:    "mailto:user@example.com",
			expected: false,
		},
		{
			name:     "tel scheme",
			input:    "tel:+1234567890",
			expected: false,
		},
		{
			name:     "javascript scheme",
			input:    "javascript:void(0)",
			expected: false,
		},
		{
			name:     "ftp scheme",
			input:    "ftp://files.example.com",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsHTTPScheme(tt.input)
			if got != tt.expected {
				t.Errorf("IsHTTPScheme(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestResolveReference(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		ref      string
		expected string
		wantErr  bool
	}{
		{
			name:     "absolute URL returned as-is",
			base:     "https://example.com",
			ref:      "https://other.com/page",
			expected: "https://other.com/page",
			wantErr:  false,
		},
		{
			name:     "relative path resolved",
			base:     "https://example.com/blog/",
			ref:      "post1",
			expected: "https://example.com/blog/post1",
			wantErr:  false,
		},
		{
			name:     "root-relative resolved",
			base:     "https://example.com/blog/",
			ref:      "/about",
			expected: "https://example.com/about",
			wantErr:  false,
		},
		{
			name:     "protocol-relative",
			base:     "https://example.com",
			ref:      "//cdn.example.com/file",
			expected: "https://cdn.example.com/file",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveReference(tt.base, tt.ref)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveReference() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("ResolveReference(%q, %q) = %v, want %v", tt.base, tt.ref, got, tt.expected)
			}
		})
	}
}
