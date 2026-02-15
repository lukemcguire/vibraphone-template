package crawler

import (
	"net/url"
	"strings"
	"testing"
)

func TestExtractLinks(t *testing.T) {
	baseURL, _ := url.Parse("https://example.com")

	tests := []struct {
		name     string
		html     string
		expected []string
	}{
		{
			name:     "extracts absolute link",
			html:     `<a href="https://example.com/page">Link</a>`,
			expected: []string{"https://example.com/page"},
		},
		{
			name:     "resolves relative link",
			html:     `<a href="/about">About</a>`,
			expected: []string{"https://example.com/about"},
		},
		{
			name:     "filters mailto scheme",
			html:     `<a href="mailto:user@example.com">Email</a>`,
			expected: []string{},
		},
		{
			name:     "filters javascript scheme",
			html:     `<a href="javascript:void(0)">Click</a>`,
			expected: []string{},
		},
		{
			name:     "handles empty href",
			html:     `<a href="">Empty</a>`,
			expected: []string{"https://example.com"},
		},
		{
			name: "extracts multiple links",
			html: `<a href="/page1">Page 1</a>
			       <a href="/page2">Page 2</a>
			       <a href="https://other.com">External</a>`,
			expected: []string{"https://example.com/page1", "https://example.com/page2", "https://other.com"},
		},
		{
			name: "deduplicates within page",
			html: `<a href="/page">Link 1</a>
			       <a href="/page">Link 2</a>
			       <a href="/page">Link 3</a>`,
			expected: []string{"https://example.com/page"},
		},
		{
			name:     "handles malformed HTML gracefully",
			html:     `<a href="/unclosed">Unclosed`,
			expected: []string{"https://example.com/unclosed"},
		},
		{
			name:     "resolves relative path without leading slash",
			html:     `<a href="contact">Contact</a>`,
			expected: []string{"https://example.com/contact"},
		},
		{
			name:     "filters ftp scheme",
			html:     `<a href="ftp://files.example.com">FTP</a>`,
			expected: []string{},
		},
		{
			name:     "normalizes URLs (lowercases scheme/host, strips fragment)",
			html:     `<a href="https://Example.com/Page#section">Fragment</a>`,
			expected: []string{"https://example.com/Page"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			links, err := ExtractLinks(strings.NewReader(tt.html), baseURL)
			if err != nil {
				t.Fatalf("ExtractLinks returned error: %v", err)
			}

			if len(links) != len(tt.expected) {
				t.Errorf("expected %d links, got %d: %v", len(tt.expected), len(links), links)
				return
			}

			// For dedup test, check that the expected link is present
			for _, expected := range tt.expected {
				found := false
				for _, link := range links {
					if link == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected link %q not found in results %v", expected, links)
				}
			}
		})
	}
}

func TestExtractLinksEmptyInput(t *testing.T) {
	baseURL, _ := url.Parse("https://example.com")

	links, err := ExtractLinks(strings.NewReader(""), baseURL)
	if err != nil {
		t.Fatalf("ExtractLinks returned error for empty input: %v", err)
	}

	if len(links) != 0 {
		t.Errorf("expected 0 links for empty input, got %d", len(links))
	}
}
