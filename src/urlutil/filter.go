package urlutil

import (
	"fmt"
	"net/url"
	"strings"
)

// IsSameDomain checks if targetURL belongs to the same domain as baseHost.
// Subdomains are considered same-domain (e.g., blog.example.com matches example.com).
func IsSameDomain(targetURL string, baseHost string) bool {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return false
	}

	host := parsed.Hostname()
	baseHost = strings.ToLower(baseHost)
	host = strings.ToLower(host)

	return host == baseHost || strings.HasSuffix(host, "."+baseHost)
}

// IsHTTPScheme returns true if the URL has an http or https scheme.
// Returns false for empty strings, non-HTTP schemes, or unparseable URLs.
func IsHTTPScheme(rawURL string) bool {
	if rawURL == "" {
		return false
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	scheme := strings.ToLower(parsed.Scheme)
	return scheme == "http" || scheme == "https"
}

// ResolveReference resolves a possibly-relative ref URL against a base URL.
// If ref is absolute, it is returned as-is. Otherwise it is resolved
// relative to base using net/url.URL.ResolveReference.
func ResolveReference(base string, ref string) (string, error) {
	baseURL, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse base URL %q: %w", base, err)
	}

	refURL, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("parse ref URL %q: %w", ref, err)
	}

	resolved := baseURL.ResolveReference(refURL)
	return resolved.String(), nil
}
