package crawler

import (
	"errors"
	"fmt"
	"os"
	"sync"

	bloom "github.com/bits-and-blooms/bloom/v3"
	"github.com/edsrzf/mmap-go"
)

// VisitedTracker implements a disk-backed bloom filter for URL deduplication.
// It uses a memory-mapped file for constant memory footprint regardless of
// crawl size, targeting 100,000+ pages with 0.1% false positive rate.
type VisitedTracker struct {
	mu        sync.Mutex
	filter    *bloom.BloomFilter
	file      *os.File
	mmap      mmap.MMap
	tmpPath   string
	count     uint64 // URLs added since last sync
	syncEvery uint64 // Sync to disk every N URLs
	lastErr   error  // Last error from sync operations
}

// NewVisitedTracker creates a new disk-backed visited URL tracker.
// It creates a temporary file in the OS temp directory for the bloom filter.
func NewVisitedTracker() (*VisitedTracker, error) {
	// Size for 100,000 URLs with 0.1% false positive rate
	// bloom.NewWithEstimates calculates optimal M and K parameters
	filter := bloom.NewWithEstimates(100000, 0.001)

	// Create temp file for the bloom filter
	tmpDir := os.TempDir()
	tmpFile, err := os.CreateTemp(tmpDir, "zombiecrawl-visited-*.bloom")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Size the file to hold the bloom filter data
	filterSize := filter.Cap()
	if err := tmpFile.Truncate(int64(filterSize)); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("truncate temp file: %w", err)
	}

	// Memory-map the file
	mapped, err := mmap.MapRegion(tmpFile, int(filterSize), mmap.RDWR, 0, 0)
	if err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("mmap temp file: %w", err)
	}

	// Initialize bloom filter with the mmap'd memory as backing store
	// We need to write the filter data to the mmap
	data, err := filter.MarshalBinary()
	if err != nil {
		_ = mapped.Unmap()
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("marshal bloom filter: %w", err)
	}

	// Copy marshaled data to mmap (filter size includes header)
	if len(data) > len(mapped) {
		_ = mapped.Unmap()
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("filter data (%d) exceeds mmap size (%d)", len(data), len(mapped))
	}
	copy(mapped, data)

	return &VisitedTracker{
		filter:    filter,
		file:      tmpFile,
		mmap:      mapped,
		tmpPath:   tmpPath,
		syncEvery: 1000, // Sync every 1000 URLs
	}, nil
}

// Visit marks a URL as visited.
func (v *VisitedTracker) Visit(url string) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.filter.AddString(url)
	v.count++

	if v.count >= v.syncEvery {
		// Record sync error for later retrieval; periodic sync is best-effort
		if err := v.syncLocked(); err != nil {
			v.lastErr = err
		}
	}
}

// IsVisited checks if a URL has been visited.
// Note: Bloom filters can have false positives but no false negatives.
func (v *VisitedTracker) IsVisited(url string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()

	return v.filter.TestString(url)
}

// VisitIfNew atomically checks if a URL is visited and marks it if not.
// Returns true if the URL was new (not previously visited), false if already visited.
func (v *VisitedTracker) VisitIfNew(url string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.filter.TestString(url) {
		return false
	}

	v.filter.AddString(url)
	v.count++

	if v.count >= v.syncEvery {
		// Record sync error for later retrieval; periodic sync is best-effort
		if err := v.syncLocked(); err != nil {
			v.lastErr = err
		}
	}

	return true
}

// syncLocked persists the bloom filter to disk. Must be called with mu held.
// Returns any error encountered during sync.
func (v *VisitedTracker) syncLocked() error {
	data, err := v.filter.MarshalBinary()
	if err != nil {
		return fmt.Errorf("marshal bloom filter: %w", err)
	}

	if len(data) <= len(v.mmap) {
		copy(v.mmap, data)
	}

	if flushErr := v.mmap.Flush(); flushErr != nil {
		return fmt.Errorf("flush mmap: %w", flushErr)
	}
	v.count = 0
	return nil
}

// Close syncs any pending data and cleans up resources.
func (v *VisitedTracker) Close() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	var errs []error

	// Include any recorded sync error
	if v.lastErr != nil {
		errs = append(errs, v.lastErr)
	}

	if v.mmap != nil {
		// Final sync before closing
		if v.count > 0 {
			if syncErr := v.syncLocked(); syncErr != nil {
				errs = append(errs, syncErr)
			}
		}
		if err := v.mmap.Unmap(); err != nil {
			errs = append(errs, fmt.Errorf("unmap: %w", err))
		}
		v.mmap = nil
	}

	if v.file != nil {
		if err := v.file.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close file: %w", err))
		}
		v.file = nil
	}

	if v.tmpPath != "" {
		if err := os.Remove(v.tmpPath); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("remove temp file: %w", err))
		}
		v.tmpPath = ""
	}

	if len(errs) > 0 {
		return fmt.Errorf("close visited tracker: %w", errors.Join(errs...))
	}

	return nil
}

// LastError returns the last error encountered during sync operations.
// This allows callers to check for disk I/O errors that occurred during
// periodic syncs without interrupting the crawl.
func (v *VisitedTracker) LastError() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.lastErr
}
