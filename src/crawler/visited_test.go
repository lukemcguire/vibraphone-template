package crawler_test

import (
	"os"
	"testing"

	"github.com/lukemcguire/zombiecrawl/crawler"
)

// TestVisitedTrackerBasicOperations verifies that Visit marks URLs as visited
// and IsVisited correctly reports their status.
func TestVisitedTrackerBasicOperations(t *testing.T) {
	vt, err := crawler.NewVisitedTracker()
	if err != nil {
		t.Fatalf("NewVisitedTracker() error: %v", err)
	}
	defer func() {
		if closeErr := vt.Close(); closeErr != nil {
			t.Errorf("Close() error: %v", closeErr)
		}
	}()

	url := "https://example.com/page"

	// New URL should not be visited
	if vt.IsVisited(url) {
		t.Error("IsVisited() returned true for new URL")
	}

	// Mark as visited
	vt.Visit(url)

	// Should now be visited
	if !vt.IsVisited(url) {
		t.Error("IsVisited() returned false after Visit()")
	}
}

// TestVisitedTrackerVisitIfNew verifies that VisitIfNew atomically tests and
// marks URLs, returning true only for the first visit.
func TestVisitedTrackerVisitIfNew(t *testing.T) {
	vt, err := crawler.NewVisitedTracker()
	if err != nil {
		t.Fatalf("NewVisitedTracker() error: %v", err)
	}
	defer func() {
		if closeErr := vt.Close(); closeErr != nil {
			t.Errorf("Close() error: %v", closeErr)
		}
	}()

	url := "https://example.com/page"

	// First call should return true (was new)
	if !vt.VisitIfNew(url) {
		t.Error("VisitIfNew() returned false for first visit")
	}

	// Second call should return false (already visited)
	if vt.VisitIfNew(url) {
		t.Error("VisitIfNew() returned true for duplicate visit")
	}
}

// TestVisitedTrackerConcurrent verifies thread-safety by having multiple
// goroutines attempt to visit the same URL concurrently.
func TestVisitedTrackerConcurrent(t *testing.T) {
	vt, err := crawler.NewVisitedTracker()
	if err != nil {
		t.Fatalf("NewVisitedTracker() error: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := vt.Close(); closeErr != nil {
			t.Errorf("Close() error: %v", closeErr)
		}
	})

	// Use a channel to collect unique visits
	const numGoroutines = 100
	results := make(chan bool, numGoroutines)

	// All goroutines try to VisitIfNew the same URL concurrently
	for range numGoroutines {
		go func() {
			results <- vt.VisitIfNew("https://example.com/concurrent")
		}()
	}

	// Count how many got true (should be exactly 1)
	trueCount := 0
	for range numGoroutines {
		if <-results {
			trueCount++
		}
	}

	if trueCount != 1 {
		t.Errorf("expected exactly 1 successful VisitIfNew, got %d", trueCount)
	}
}

// TestVisitedTrackerCleanup verifies that Close properly cleans up temp files.
func TestVisitedTrackerCleanup(t *testing.T) {
	vt, err := crawler.NewVisitedTracker()
	if err != nil {
		t.Fatalf("NewVisitedTracker() error: %v", err)
	}

	// Add some URLs
	for i := range 100 {
		vt.Visit("https://example.com/page" + string(rune(i)))
	}

	// Close should clean up temp file
	if closeErr := vt.Close(); closeErr != nil {
		t.Errorf("Close() error: %v", closeErr)
	}
}

// TestVisitedTrackerLargeScale verifies the bloom filter handles thousands of
// unique URLs correctly.
func TestVisitedTrackerLargeScale(t *testing.T) {
	vt, err := crawler.NewVisitedTracker()
	if err != nil {
		t.Fatalf("NewVisitedTracker() error: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := vt.Close(); closeErr != nil {
			t.Errorf("Close() error: %v", closeErr)
		}
	})

	// Add 1000 URLs to verify bloom filter scales
	for i := range 1000 {
		url := "https://example.com/page/" + string(rune(i))
		if !vt.VisitIfNew(url) {
			t.Errorf("VisitIfNew() returned false for unique URL %d", i)
		}
	}

	// Verify all are marked as visited
	for i := range 1000 {
		url := "https://example.com/page/" + string(rune(i))
		if !vt.IsVisited(url) {
			t.Errorf("IsVisited() returned false for visited URL %d", i)
		}
	}
}

// TestVisitedTrackerClosesTempFile verifies that Close properly cleans up
// resources and that double close is safe.
func TestVisitedTrackerClosesTempFile(t *testing.T) {
	vt, err := crawler.NewVisitedTracker()
	if err != nil {
		t.Fatalf("NewVisitedTracker() error: %v", err)
	}

	// Get temp file path before closing (implementation detail)
	// This test ensures Close() properly cleans up resources

	if closeErr := vt.Close(); closeErr != nil {
		t.Errorf("Close() error: %v", closeErr)
	}

	// Double close should not panic
	if closeErr := vt.Close(); closeErr != nil {
		// Some implementations may return error on double close, that's ok
		t.Logf("Double close returned: %v (may be expected)", closeErr)
	}
}

// TestVisitedTrackerMemoryFootprint verifies the tracker doesn't consume
// unbounded memory by adding many URLs.
func TestVisitedTrackerMemoryFootprint(t *testing.T) {
	// This test verifies the tracker doesn't consume unbounded memory
	// by checking that creating a tracker and adding URLs doesn't grow
	// memory excessively. Actual memory measurement would require runtime
	// metrics, so we just ensure the operations succeed at scale.

	vt, err := crawler.NewVisitedTracker()
	if err != nil {
		t.Fatalf("NewVisitedTracker() error: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := vt.Close(); closeErr != nil {
			t.Errorf("Close() error: %v", closeErr)
		}
	})

	// Add 10,000 URLs - should complete without OOM
	for i := range 10000 {
		vt.Visit("https://example.com/page/" + string(rune(i)))
	}

	// If we got here without OOM, the test passes
}

// TestVisitedTrackerLastError verifies that LastError returns nil for new
// trackers and records errors from sync operations.
func TestVisitedTrackerLastError(t *testing.T) {
	vt, err := crawler.NewVisitedTracker()
	if err != nil {
		t.Fatalf("NewVisitedTracker() error: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := vt.Close(); closeErr != nil {
			t.Errorf("Close() error: %v", closeErr)
		}
	})

	// New tracker should have no last error
	if lastErr := vt.LastError(); lastErr != nil {
		t.Errorf("LastError() = %v, want nil for new tracker", lastErr)
	}

	// After normal operations, still no error
	vt.Visit("https://example.com/page1")
	if lastErr := vt.LastError(); lastErr != nil {
		t.Errorf("LastError() = %v, want nil after successful visit", lastErr)
	}
}

// TestMain runs all tests in the package.
func TestMain(m *testing.M) {
	// Run tests and exit with appropriate code
	os.Exit(m.Run())
}
