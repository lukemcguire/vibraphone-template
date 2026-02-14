package urlutil

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "fragment stripping",
			input:    "https://example.com/page#section",
			expected: "https://example.com/page",
			wantErr:  false,
		},
		{
			name:     "trailing slash stripping",
			input:    "https://example.com/about/",
			expected: "https://example.com/about",
			wantErr:  false,
		},
		{
			name:     "root path keeps slash",
			input:    "https://example.com/",
			expected: "https://example.com/",
			wantErr:  false,
		},
		{
			name:     "query params preserved",
			input:    "https://example.com/search?q=foo",
			expected: "https://example.com/search?q=foo",
			wantErr:  false,
		},
		{
			name:     "scheme lowercased",
			input:    "HTTPS://Example.Com/Page",
			expected: "https://example.com/Page",
			wantErr:  false,
		},
		{
			name:     "already normalized URL passes through",
			input:    "https://example.com/path",
			expected: "https://example.com/path",
			wantErr:  false,
		},
		{
			name:     "empty string returns error",
			input:    "",
			expected: "",
			wantErr:  true,
		},
		{
			name:     "invalid URL returns error",
			input:    "://invalid",
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Normalize(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Normalize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("Normalize() = %v, want %v", got, tt.expected)
			}
		})
	}
}
