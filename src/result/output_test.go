package result

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	links := []LinkResult{
		{
			URL:           "https://example.com/broken",
			StatusCode:    404,
			Error:         "not found",
			ErrorCategory: Category4xx,
			SourcePage:    "https://example.com/",
			IsExternal:    false,
		},
		{
			URL:           "https://external.com/error",
			StatusCode:    0,
			Error:         "connection refused",
			ErrorCategory: CategoryConnectionRefused,
			SourcePage:    "https://example.com/",
			IsExternal:    true,
		},
	}

	var buf bytes.Buffer
	err := WriteJSON(&buf, links)
	if err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}

	// Verify output is valid JSON
	var decoded []LinkResult
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("Output is not valid JSON: %v", err)
	}

	if len(decoded) != 2 {
		t.Errorf("Expected 2 links, got %d", len(decoded))
	}

	// Verify field names are snake_case
	var raw []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	if _, ok := raw[0]["url"]; !ok {
		t.Error("Expected 'url' field in JSON output")
	}
	if _, ok := raw[0]["status_code"]; !ok {
		t.Error("Expected 'status_code' field in JSON output")
	}
	if _, ok := raw[0]["error_type"]; !ok {
		t.Error("Expected 'error_type' field in JSON output")
	}
	if _, ok := raw[0]["source_page"]; !ok {
		t.Error("Expected 'source_page' field in JSON output")
	}
	if _, ok := raw[0]["is_external"]; !ok {
		t.Error("Expected 'is_external' field in JSON output")
	}

	// Verify URLs are not HTML-escaped
	if !strings.Contains(buf.String(), "https://example.com/broken") {
		t.Error("URLs should not be HTML-escaped")
	}
}

func TestWriteJSON_Empty(t *testing.T) {
	links := []LinkResult{}

	var buf bytes.Buffer
	err := WriteJSON(&buf, links)
	if err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}

	// Should output empty array
	if !bytes.Equal(buf.Bytes(), []byte("[]\n")) {
		t.Errorf("Expected '[]\\n', got %q", buf.String())
	}
}

func TestWriteCSV(t *testing.T) {
	links := []LinkResult{
		{
			URL:           "https://example.com/broken",
			StatusCode:    404,
			ErrorCategory: Category4xx,
			SourcePage:    "https://example.com/",
			IsExternal:    false,
		},
		{
			URL:           "https://external.com/error",
			StatusCode:    0,
			ErrorCategory: CategoryConnectionRefused,
			SourcePage:    "https://example.com/",
			IsExternal:    true,
		},
	}

	var buf bytes.Buffer
	err := WriteCSV(&buf, links)
	if err != nil {
		t.Fatalf("WriteCSV returned error: %v", err)
	}

	// Parse CSV output
	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("Failed to parse CSV output: %v", err)
	}

	// Verify header
	expectedHeader := []string{"url", "status_code", "error_type", "source_page", "is_external"}
	if len(records) < 1 {
		t.Fatal("Expected at least header row")
	}
	for i, col := range expectedHeader {
		if records[0][i] != col {
			t.Errorf("Header column %d: expected %q, got %q", i, col, records[0][i])
		}
	}

	// Verify data rows
	if len(records) != 3 { // header + 2 data rows
		t.Errorf("Expected 3 records (header + 2 data), got %d", len(records))
	}

	// First data row
	if records[1][0] != "https://example.com/broken" {
		t.Errorf("Expected URL in row 1, got %q", records[1][0])
	}
	if records[1][1] != "404" {
		t.Errorf("Expected status_code '404' in row 1, got %q", records[1][1])
	}
	if records[1][2] != "4xx" {
		t.Errorf("Expected error_type '4xx' in row 1, got %q", records[1][2])
	}
	if records[1][4] != "false" {
		t.Errorf("Expected is_external 'false' in row 1, got %q", records[1][4])
	}

	// Second data row - status_code should be empty for 0
	if records[2][1] != "" {
		t.Errorf("Expected empty status_code in row 2 (status 0), got %q", records[2][1])
	}
}

func TestWriteCSV_EmptyWithHeader(t *testing.T) {
	links := []LinkResult{}

	var buf bytes.Buffer
	err := WriteCSV(&buf, links)
	if err != nil {
		t.Fatalf("WriteCSV returned error: %v", err)
	}

	// Should still have header row
	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("Failed to parse CSV output: %v", err)
	}

	if len(records) != 1 {
		t.Errorf("Expected 1 record (header only), got %d", len(records))
	}

	expectedHeader := []string{"url", "status_code", "error_type", "source_page", "is_external"}
	for i, col := range expectedHeader {
		if records[0][i] != col {
			t.Errorf("Header column %d: expected %q, got %q", i, col, records[0][i])
		}
	}
}

func TestStatusCodeStr(t *testing.T) {
	tests := []struct {
		code     int
		expected string
	}{
		{0, ""},
		{200, "200"},
		{404, "404"},
		{500, "500"},
	}

	for _, tt := range tests {
		result := statusCodeStr(tt.code)
		if result != tt.expected {
			t.Errorf("statusCodeStr(%d) = %q, expected %q", tt.code, result, tt.expected)
		}
	}
}
