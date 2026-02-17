package result

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

// WriteJSON writes the broken links as a formatted JSON array to the writer.
// Uses flat array format (not wrapped with metadata) for simpler CI integration.
func WriteJSON(w io.Writer, links []LinkResult) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(links); err != nil {
		return fmt.Errorf("write json output: %w", err)
	}
	return nil
}

// WriteCSV writes the broken links as CSV to the writer.
// Always includes a header row, even if there are no broken links.
// Column order: url, status_code, error_type, source_page, is_external
func WriteCSV(w io.Writer, links []LinkResult) error {
	cw := csv.NewWriter(w)

	// Write header row
	header := []string{"url", "status_code", "error_type", "source_page", "is_external"}
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}

	// Write data rows
	for _, link := range links {
		record := []string{
			link.URL,
			statusCodeStr(link.StatusCode),
			string(link.ErrorCategory),
			link.SourcePage,
			strconv.FormatBool(link.IsExternal),
		}
		if err := cw.Write(record); err != nil {
			return fmt.Errorf("write csv record for %s: %w", link.URL, err)
		}
	}

	cw.Flush()
	if err := cw.Error(); err != nil {
		return fmt.Errorf("flush csv output: %w", err)
	}
	return nil
}

// statusCodeStr converts an HTTP status code to a string.
// Returns empty string for 0 (no HTTP status).
func statusCodeStr(code int) string {
	if code == 0 {
		return ""
	}
	return strconv.Itoa(code)
}
