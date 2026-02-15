package urlutil

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Normalize takes a raw URL string and returns a normalized version.
// Normalization includes:
// - Lowercasing the scheme and host
// - Stripping fragments (#section)
// - Stripping trailing slashes (except for root path "/")
// - Preserving query parameters
//
// Returns an error if the input is empty or cannot be parsed as a valid URL.
func Normalize(rawURL string) (string, error) {
	// Empty string is invalid
	if rawURL == "" {
		return "", errors.New("cannot normalize empty URL")
	}

	// Parse the URL
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("normalize URL %q: %w", rawURL, err)
	}

	// Validate that we have at least a scheme and host
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("URL must have both scheme and host")
	}

	// Lowercase the scheme and host
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)

	// Strip the fragment
	parsed.Fragment = ""

	// Strip trailing slash from path, unless it's the root path "/"
	if parsed.Path != "/" && strings.HasSuffix(parsed.Path, "/") {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	}

	// Return the normalized URL string
	return parsed.String(), nil
}
