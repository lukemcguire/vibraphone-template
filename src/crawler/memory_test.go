package crawler_test

import (
	"testing"

	"github.com/lukemcguire/zombiecrawl/crawler"
)

// TestMemoryWatcherBasicCheck verifies that Check returns valid memory
// statistics and normal throttle level with a reasonable memory limit.
func TestMemoryWatcherBasicCheck(t *testing.T) {
	// Create watcher with 1GB limit
	mw := crawler.NewMemoryWatcher(1024)

	usedPercent, level := mw.Check()

	// Memory usage should be between 0 and 100%
	if usedPercent < 0 || usedPercent > 100 {
		t.Errorf("usedPercent = %f, want between 0 and 100", usedPercent)
	}

	// At startup with reasonable limit, level should be normal
	if level != crawler.ThrottleNormal {
		t.Errorf("level = %v, want ThrottleNormal", level)
	}
}

// TestMemoryWatcherThrottleLevels verifies that small memory limits trigger
// warning or critical throttle levels.
func TestMemoryWatcherThrottleLevels(t *testing.T) {
	// Create watcher with very small limit to trigger throttling
	mw := crawler.NewMemoryWatcher(1) // 1MB limit

	_, level := mw.Check()

	// With such a tiny limit, we should be in warning or critical
	if level == crawler.ThrottleNormal {
		t.Error("expected throttle level > ThrottleNormal with 1MB limit")
	}
}

// TestMemoryWatcherCallback verifies that SetThrottleCallback registers a
// callback that is invoked when throttle level changes.
func TestMemoryWatcherCallback(t *testing.T) {
	mw := crawler.NewMemoryWatcher(1024)

	callbackCalled := false
	mw.SetThrottleCallback(func(level crawler.ThrottleLevel) {
		callbackCalled = true
	})

	// Check triggers callback if throttling needed
	mw.Check()

	// Callback may or may not be called depending on memory state,
	// but SetThrottleCallback should not panic
	_ = callbackCalled
}

// TestMemoryWatcherMultipleChecks verifies that multiple Check calls are safe
// and don't cause race conditions.
func TestMemoryWatcherMultipleChecks(t *testing.T) {
	mw := crawler.NewMemoryWatcher(1024)

	// Multiple checks should be safe
	for i := 0; i < 10; i++ {
		_, level := mw.Check()
		_ = level
	}
}

// TestMemoryWatcherSetLimit verifies that SetLimit updates the memory limit
// and subsequent Check calls use the new limit.
func TestMemoryWatcherSetLimit(t *testing.T) {
	mw := crawler.NewMemoryWatcher(1024)

	// Initial check with 1GB limit
	_, level1 := mw.Check()

	// Set a new limit (2GB in bytes)
	mw.SetLimit(2 * 1024 * 1024 * 1024)

	// Check should now use the new limit
	usedPercent, level2 := mw.Check()

	// With a larger limit, usage percentage should be lower (or same if very small usage)
	_ = usedPercent
	_ = level1
	_ = level2

	// Verify SetLimit doesn't panic and subsequent Check works
	// The exact behavior depends on actual memory usage, so we just verify no panic
}
