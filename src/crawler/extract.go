package crawler

import (
	"fmt"
	"io"
	"net/url"

	"github.com/lukemcguire/zombiecrawl/urlutil"
	"golang.org/x/net/html"
)

// ExtractLinks parses HTML from the given reader and extracts all anchor tag hrefs.
// It resolves relative URLs against the baseURL, filters non-HTTP schemes,
// normalizes each URL, and returns a deduplicated list of absolute URLs.
func ExtractLinks(body io.Reader, baseURL *url.URL) ([]string, error) {
	tokenizer := html.NewTokenizer(body)
	seen := make(map[string]bool)
	var links []string
	var errs []error

	for {
		tokenType := tokenizer.Next()
		switch tokenType {
		case html.ErrorToken:
			// End of document or error
			if len(errs) > 0 {
				return links, fmt.Errorf("encountered %d parse errors (first: %w)", len(errs), errs[0])
			}
			return links, nil
		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			if token.Data == "a" {
				for _, attr := range token.Attr {
					if attr.Key == "href" {
						href := attr.Val
						if href == "" {
							// Empty href points to current page
							href = baseURL.String()
						}

						// Resolve relative URL against base
						hrefURL, err := url.Parse(href)
						if err != nil {
							errs = append(errs, fmt.Errorf("parse href %q: %w", href, err))
							continue
						}
						resolved := baseURL.ResolveReference(hrefURL)

						resolvedStr := resolved.String()

						// Filter non-HTTP schemes
						if !urlutil.IsHTTPScheme(resolvedStr) {
							continue
						}

						// Normalize the URL
						normalized, err := urlutil.Normalize(resolvedStr)
						if err != nil {
							errs = append(errs, fmt.Errorf("normalize URL %q: %w", resolvedStr, err))
							continue
						}

						// Deduplicate
						if !seen[normalized] {
							seen[normalized] = true
							links = append(links, normalized)
						}
					}
				}
			}
		}
	}
}
