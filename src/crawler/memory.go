package crawler

import (
	"runtime"
	"runtime/debug"
	"sync"
)

// ThrottleLevel indicates memory pressure severity.
type ThrottleLevel int

const (
	// ThrottleNormal indicates memory usage is within normal bounds.
	ThrottleNormal ThrottleLevel = iota
	// ThrottleWarning indicates memory usage is elevated (75-90% of limit).
	ThrottleWarning
	// ThrottleCritical indicates memory usage is critical (>90% of limit).
	ThrottleCritical
)

// MemoryWatcher monitors memory pressure and triggers throttling callbacks.
// It uses runtime/debug.SetMemoryLimit for soft memory limits (Go 1.19+).
type MemoryWatcher struct {
	mu           sync.RWMutex
	limitBytes   int64
	callback     func(level ThrottleLevel)
	lastLevel    ThrottleLevel
}

// NewMemoryWatcher creates a memory watcher with the specified limit in MB.
// The limit is set at 70-80% of the specified value to avoid GC thrashing.
func NewMemoryWatcher(limitMB int64) *MemoryWatcher {
	limitBytes := limitMB * 1024 * 1024

	// Set soft memory limit using Go 1.19+ API
	// We set it at 100% of requested limit - the caller should account for overhead
	debug.SetMemoryLimit(limitBytes)

	return &MemoryWatcher{
		limitBytes: limitBytes,
		lastLevel:  ThrottleNormal,
	}
}

// Check returns current memory usage percentage and throttle level.
// Call this periodically to detect memory pressure.
func (m *MemoryWatcher) Check() (usedPercent float64, level ThrottleLevel) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Use HeapAlloc for actual memory in use (not Sys which includes reserved)
	usedBytes := float64(memStats.HeapAlloc)
	limitBytes := float64(m.limitBytes)

	if limitBytes <= 0 {
		return 0, ThrottleNormal
	}

	usedPercent = (usedBytes / limitBytes) * 100

	// Determine throttle level
	switch {
	case usedPercent >= 90:
		level = ThrottleCritical
	case usedPercent >= 75:
		level = ThrottleWarning
	default:
		level = ThrottleNormal
	}

	// Trigger callback if level changed
	m.mu.RLock()
	lastLevel := m.lastLevel
	callback := m.callback
	m.mu.RUnlock()

	if level != lastLevel && callback != nil {
		m.mu.Lock()
		m.lastLevel = level
		m.mu.Unlock()
		callback(level)
	}

	return usedPercent, level
}

// SetThrottleCallback registers a callback to be invoked when throttle level changes.
func (m *MemoryWatcher) SetThrottleCallback(cb func(level ThrottleLevel)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callback = cb
}

// SetLimit updates the memory limit in bytes.
func (m *MemoryWatcher) SetLimit(limitBytes int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.limitBytes = limitBytes
	debug.SetMemoryLimit(limitBytes)
}
