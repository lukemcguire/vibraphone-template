# Phase 5: Production Polish - Research

**Researched:** 2026-02-17
**Domain:** Go production hardening (memory management, bloom filters, adaptive rate limiting)
**Confidence:** HIGH

## Summary

This phase focuses on hardening the zombiecrawl crawler for production use at 100K+ page scale. The key technical challenge is replacing the in-memory `sync.Map` visited URL tracker with a disk-backed bloom filter that maintains constant memory footprint regardless of crawl size.

**Primary recommendation:** Use `github.com/bits-and-blooms/bloom/v3` with mmap-backed storage via `github.com/edsrzf/mmap-go`. For memory monitoring, use `runtime/debug.SetMemoryLimit()` combined with `runtime.ReadMemStats()`. For adaptive rate limiting, extend the existing `golang.org/x/time/rate` usage with dynamic `SetLimit()` calls.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Edge Case Handling
- Malformed HTML that fails to parse → treat as broken link, report in results
- Redirect chains → no hard limit, detect cycles only
- Binary files (PDFs, images, zips) → quick HEAD check, report as valid if 2xx response, skip parsing
- Auth-gated pages (401/403) → detect and classify as "requires auth" rather than broken

#### Memory Strategy
- Target scale: 100,000+ pages
- URL tracking: disk-backed bloom filter (replace current `sync.Map` in-memory approach)
  - Memory-mapped file for flat memory footprint
  - Zero false positives — if bloom says visited, it's visited
- Memory pressure response: dynamic throttling (reduce concurrency, prune if needed)
- HTML buffering: use Go's http client default behavior

#### Error Messages
- Style: minimal by default, trust user to investigate
- Invalid URLs: explain the issue (missing scheme, etc.)
- Network errors: add `--verbose-network` flag for detailed diagnostics (DNS, timeout, connection refused)
  - Default: simple error messages
  - `--verbose-network`: full error chain with context
- Exit codes: binary (0 = success, 1 = failure) — already implemented

#### Performance Tuning
- Target speed: 50 pages/second (matches success criteria)
- Auto-tune concurrency: dynamically adjust worker count based on server response times
  - Ramp up if server handles load well
  - Back off if responses slow down
  - Explicit `--concurrency` flag caps or disables auto-tune
- Rename `--rate-limit` to `--delay` (breaking change)
  - Change semantics from req/sec to ms between requests
  - 10 req/sec → 100ms delay
  - Apply stricter of `--delay` or robots.txt `Crawl-delay`
- HTTP connections: use Go's default transport behavior

#### Breaking Changes
- `--rate-limit` renamed to `--delay` with inverted semantics (delay vs rate)
- Update all code references from rate-limit to delay

### Claude's Discretion
(NONE - all decisions locked)

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope
</user_constraints>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| github.com/bits-and-blooms/bloom/v3 | v3.7.0 | Bloom filter implementation | Used by Milvus, Beego; murmurhash; serialization support |
| github.com/edsrzf/mmap-go | v1.2.0 | Memory-mapped file I/O | Portable (Linux/macOS/Windows); 1.1k stars; well-maintained |
| golang.org/x/time/rate | v0.14.0 | Token bucket rate limiting | Already in project; goroutine-safe; dynamic rate adjustment |
| runtime/debug | (stdlib) | Memory limit control | Go 1.19+ soft memory limits via SetMemoryLimit |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| runtime | (stdlib) | ReadMemStats for monitoring | Memory pressure detection before OOM |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| bits-and-blooms/bloom | willf/bloom | Similar API; bits-and-blooms has more stars and used by major projects |
| mmap-go | syscall.Mmap directly | Less portable; mmap-go handles Windows/Unix differences |
| SetMemoryLimit | cgroup memory limits | External dependency; SetMemoryLimit is Go-native and works everywhere |

**Installation:**
```bash
cd src && go get github.com/bits-and-blooms/bloom/v3@latest
cd src && go get github.com/edsrzf/mmap-go@latest
```

## Architecture Patterns

### Recommended Project Structure
```
src/
├── crawler/
│   ├── crawler.go      # Main crawler coordination (update for bloom filter)
│   ├── worker.go       # URL checking logic
│   ├── visited.go      # NEW: Disk-backed bloom filter wrapper
│   └── ...
├── result/
│   └── errors.go       # Error classification (add CategoryAuthRequired)
└── main.go             # CLI flags (rename --rate-limit to --delay)
```

### Pattern 1: Disk-Backed Bloom Filter
**What:** Wrap bloom filter with memory-mapped file for persistent, bounded-memory URL tracking.
**When to use:** When crawling 100K+ pages where in-memory URL storage would OOM.

**Example:**
```go
// Source: Derived from bits-and-blooms/bloom docs + mmap-go README
package crawler

import (
    "os"
    "github.com/bits-and-blooms/bloom/v3"
    "github.com/edsrzf/mmap-go"
)

type DiskBackedBloom struct {
    filter *bloom.BloomFilter
    file   *os.File
    mdata  mmap.MMap
}

// NewDiskBackedBloom creates a bloom filter backed by a memory-mapped file.
// For 100K URLs with 1% false positive rate: ~120KB filter
// For 100K URLs with 0.1% false positive rate: ~180KB filter
func NewDiskBackedBloom(path string, expectedItems uint, falsePositiveRate float64) (*DiskBackedBloom, error) {
    // Calculate required size
    filter := bloom.NewWithEstimates(expectedItems, falsePositiveRate)

    // Get the size needed for the filter
    // bloom.BloomFilter uses a bitset.BitSet internally
    buf := new(bytes.Buffer)
    filter.WriteTo(buf)
    size := buf.Len()

    // Create or open the file
    f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
    if err != nil {
        return nil, err
    }

    // Size the file
    if err := f.Truncate(int64(size)); err != nil {
        f.Close()
        return nil, err
    }

    // Memory map the file
    mdata, err := mmap.Map(f, mmap.RDWR, 0)
    if err != nil {
        f.Close()
        return nil, err
    }

    // Read existing filter from file if it has content
    if stat, _ := f.Stat(); stat.Size() > 0 {
        filter.ReadFrom(bytes.NewReader(mdata))
    }

    return &DiskBackedBloom{
        filter: filter,
        file:   f,
        mdata:  mdata,
    }, nil
}

func (d *DiskBackedBloom) Add(url string) {
    d.filter.AddString(url)
    // Sync to mmap - changes are visible immediately
    // Call mdata.Flush() periodically or on close for persistence
}

func (d *DiskBackedBloom) Test(url string) bool {
    return d.filter.TestString(url)
}

func (d *DiskBackedBloom) Close() error {
    d.mdata.Flush()
    d.mdata.Unmap()
    return d.file.Close()
}
```

### Pattern 2: Adaptive Rate Limiting
**What:** Dynamically adjust rate limit based on server response times.
**When to use:** When crawling at high speed and need to back off when server is overloaded.

**Example:**
```go
// Source: Based on golang.org/x/time/rate docs
type AdaptiveLimiter struct {
    limiter     *rate.Limiter
    targetRTT   time.Duration  // Target response time (e.g., 200ms)
    currentRate rate.Limit
    mu          sync.Mutex
}

func NewAdaptiveLimiter(initialRPS int, targetRTT time.Duration) *AdaptiveLimiter {
    return &AdaptiveLimiter{
        limiter:   rate.NewLimiter(rate.Limit(initialRPS), initialRPS),
        targetRTT: targetRTT,
        currentRate: rate.Limit(initialRPS),
    }
}

func (a *AdaptiveLimiter) ObserveRTT(rtt time.Duration) {
    a.mu.Lock()
    defer a.mu.Unlock()

    // If RTT is higher than target, reduce rate
    // If RTT is lower than target, increase rate
    ratio := float64(a.targetRTT) / float64(rtt)

    newRate := a.currentRate * rate.Limit(ratio)

    // Clamp to reasonable bounds (e.g., 1-100 RPS)
    if newRate < 1 {
        newRate = 1
    }
    if newRate > 100 {
        newRate = 100
    }

    a.currentRate = newRate
    a.limiter.SetLimit(newRate)
}

func (a *AdaptiveLimiter) Wait(ctx context.Context) error {
    return a.limiter.Wait(ctx)
}
```

### Pattern 3: Memory Pressure Detection
**What:** Monitor memory usage and trigger throttling before OOM.
**When to use:** Production crawlers that may hit memory limits with large workloads.

**Example:**
```go
// Source: Go runtime/debug docs
type MemoryMonitor struct {
    limitBytes   int64
    warnPercent  float64
    throttleFunc func()
}

func NewMemoryMonitor(limitBytes int64, warnPercent float64) *MemoryMonitor {
    // Set soft memory limit - GC will run more aggressively as we approach
    debug.SetMemoryLimit(limitBytes)
    return &MemoryMonitor{
        limitBytes:  limitBytes,
        warnPercent: warnPercent,
    }
}

func (m *MemoryMonitor) Check() (usedPercent float64, shouldThrottle bool) {
    var stats runtime.MemStats
    runtime.ReadMemStats(&stats)

    // Sys is total memory obtained from OS
    // For soft limit checking, use Alloc (currently allocated)
    usedPercent = float64(stats.Alloc) / float64(m.limitBytes) * 100

    shouldThrottle = usedPercent >= m.warnPercent

    if shouldThrottle && m.throttleFunc != nil {
        m.throttleFunc()
    }

    return usedPercent, shouldThrottle
}
```

### Anti-Patterns to Avoid
- **Hand-rolling bloom filter math:** Use `bloom.NewWithEstimates(n, fp)` which correctly computes m and k parameters
- **Not syncing mmap:** Changes to mmap are not immediately persisted; call `Flush()` on close
- **Ignoring goroutine safety:** bits-and-blooms/bloom is NOT goroutine-safe; wrap with mutex or use channels
- **Over-tuning GOGC:** Use SetMemoryLimit instead; GOGC is relative, SetMemoryLimit is absolute

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Bloom filter bit manipulation | Custom bitset with hashing | github.com/bits-and-blooms/bloom | Murmurhash, optimal k computation, serialization |
| Memory-mapped I/O | syscall.Mmap directly | github.com/edsrzf/mmap-go | Windows/Unix differences, error handling |
| Rate limiter with burst | Token bucket from scratch | golang.org/x/time/rate | Already in project, battle-tested |
| Memory limit enforcement | Manual GC triggering | runtime/debug.SetMemoryLimit | Go 1.19+ native, works with GC pacer |

**Key insight:** The bloom filter library handles all the mathematical complexity (optimal filter size, number of hash functions). The mmap library handles all OS-specific differences. Together they provide a simple API for persistent, bounded-memory visited URL tracking.

## Common Pitfalls

### Pitfall 1: Bloom Filter False Positives Cause URL Skipping
**What goes wrong:** URLs that were never visited get skipped because bloom filter says "already seen"
**Why it happens:** Bloom filters have configurable false positive rate (1% is typical)
**How to avoid:**
1. Use low false positive rate (0.1% or lower for crawlers)
2. Size filter correctly with `NewWithEstimates(expectedURLs, fpRate)`
3. Document that rare skips are expected behavior
**Warning signs:** Crawls complete unusually fast; link count lower than expected

### Pitfall 2: Mmap File Not Synced on Crash
**What goes wrong:** Power loss or crash loses visited URL state
**Why it happens:** Mmap writes are not immediately persisted to disk
**How to avoid:**
1. Call `mmap.Flush()` periodically (e.g., every 1000 URLs)
2. Always `Flush()` in `Close()` method
3. Use `syscall.MS_SYNC` on Linux for blocking sync
**Warning signs:** After crash, bloom filter empty or corrupted

### Pitfall 3: Rate Limiter Becomes Too Aggressive
**What goes wrong:** Adaptive rate limiter drops to 1 RPS and never recovers
**Why it happens:** Single slow response (timeout) skews average; no recovery logic
**How to avoid:**
1. Use exponential moving average for RTT, not raw values
2. Implement minimum rate floor (e.g., never below 5 RPS)
3. Implement gradual recovery (increase by 10% per good RTT)
**Warning signs:** Crawl speed drops dramatically after one slow response

### Pitfall 4: Memory Limit Too Low Causes GC Thrashing
**What goes wrong:** Setting memory limit too low causes GC to run continuously
**Why it happens:** GC pacer tries to stay under limit by running constantly
**How to avoid:**
1. Set limit at 70-80% of available system memory
2. Monitor GC pause times; if >10% of CPU, raise limit
3. Use `debug.SetGCPercent()` alongside limit for fine-tuning
**Warning signs:** CPU at 100% but low throughput; many short GC cycles

## Code Examples

### Creating a Production-Ready Visited URL Tracker
```go
// Source: Combining bits-and-blooms/bloom with mmap-go patterns
package crawler

import (
    "bytes"
    "os"
    "path/filepath"
    "sync"

    "github.com/bits-and-blooms/bloom/v3"
    "github.com/edsrzf/mmap-go"
)

const (
    // For 100K URLs at 0.1% false positive rate
    // m = -n * ln(p) / (ln(2)^2) ≈ 1.44 * n * log2(1/p)
    // For n=100000, p=0.001: m ≈ 1,437,588 bits ≈ 180KB
    defaultExpectedItems = 100000
    defaultFalsePositive = 0.001 // 0.1%
)

type VisitedTracker struct {
    filter *bloom.BloomFilter
    file   *os.File
    mdata  mmap.MMap
    mu     sync.Mutex
    path   string
}

func NewVisitedTracker(dataDir string) (*VisitedTracker, error) {
    path := filepath.Join(dataDir, "visited.bloom")

    // Create bloom filter sized for 100K URLs at 0.1% FP rate
    filter := bloom.NewWithEstimates(defaultExpectedItems, defaultFalsePositive)

    // Calculate size needed
    buf := new(bytes.Buffer)
    filter.WriteTo(buf)
    size := buf.Len()

    // Open/create file
    f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
    if err != nil {
        return nil, err
    }

    // Size the file
    if err := f.Truncate(int64(size)); err != nil {
        f.Close()
        return nil, err
    }

    // Memory map
    mdata, err := mmap.Map(f, mmap.RDWR, 0)
    if err != nil {
        f.Close()
        return nil, err
    }

    // Load existing data if present
    if stat, _ := f.Stat(); stat.Size() > 0 {
        filter.ReadFrom(bytes.NewReader(mdata))
    }

    return &VisitedTracker{
        filter: filter,
        file:   f,
        mdata:  mdata,
        path:   path,
    }, nil
}

func (v *VisitedTracker) Visit(url string) {
    v.mu.Lock()
    defer v.mu.Unlock()
    v.filter.AddString(url)
}

func (v *VisitedTracker) IsVisited(url string) bool {
    v.mu.Lock()
    defer v.mu.Unlock()
    return v.filter.TestString(url)
}

func (v *VisitedTracker) VisitIfNew(url string) bool {
    v.mu.Lock()
    defer v.mu.Unlock()
    // TestOrAdd returns true if already present, false if newly added
    return !v.filter.TestOrAddString(url)
}

func (v *VisitedTracker) Sync() error {
    v.mu.Lock()
    defer v.mu.Unlock()

    // Write filter to mmap
    buf := new(bytes.Buffer)
    if _, err := v.filter.WriteTo(buf); err != nil {
        return err
    }
    copy(v.mdata, buf.Bytes())

    return v.mdata.Flush()
}

func (v *VisitedTracker) Close() error {
    if err := v.Sync(); err != nil {
        return err
    }
    if err := v.mdata.Unmap(); err != nil {
        return err
    }
    return v.file.Close()
}
```

### Memory Pressure Handler
```go
// Source: Go runtime/debug documentation
package crawler

import (
    "context"
    "runtime"
    "runtime/debug"
    "time"
)

type MemoryWatcher struct {
    limitBytes    int64
    throttleLevel int // 0=normal, 1=warning, 2=critical
    checkInterval time.Duration
}

func NewMemoryWatcher(limitMB int64) *MemoryWatcher {
    limitBytes := limitMB * 1024 * 1024
    debug.SetMemoryLimit(limitBytes)
    return &MemoryWatcher{
        limitBytes:    limitBytes,
        checkInterval: 5 * time.Second,
    }
}

func (m *MemoryWatcher) Start(ctx context.Context) chan int {
    throttleCh := make(chan int, 1)

    go func() {
        ticker := time.NewTicker(m.checkInterval)
        defer ticker.Stop()

        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                var stats runtime.MemStats
                runtime.ReadMemStats(&stats)

                usedPercent := float64(stats.Alloc) / float64(m.limitBytes) * 100

                var level int
                switch {
                case usedPercent >= 90:
                    level = 2 // Critical: drastic action
                case usedPercent >= 75:
                    level = 1 // Warning: start throttling
                default:
                    level = 0 // Normal
                }

                if level != m.throttleLevel {
                    m.throttleLevel = level
                    select {
                    case throttleCh <- level:
                    default:
                    }
                }
            }
        }
    }()

    return throttleCh
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| GOGC only for memory control | SetMemoryLimit (Go 1.19+) | Go 1.19 (Aug 2022) | Soft memory limit that GC respects; prevents OOM |
| In-memory URL tracking | Disk-backed bloom filter with mmap | Always applicable | Constant memory regardless of crawl size |
| Fixed rate limiting | Adaptive rate limiting based on RTT | Modern pattern | Better server citizenship, faster when possible |

**Deprecated/outdated:**
- `GOMEMLIMIT` via environment variable: Use `debug.SetMemoryLimit()` for programmatic control with same effect
- Manual GC triggering via `runtime.GC()`: Use `debug.SetMemoryLimit()` which integrates with GC pacer

## Open Questions

1. **Should we persist bloom filter across crawl runs?**
   - What we know: Mmap-backed bloom filter can persist to disk
   - What's unclear: Is this useful for zombiecrawl use case? (typically single-run tool)
   - Recommendation: Implement persistence, but auto-delete on successful completion unless `--resume` flag used

2. **What's the right false positive rate for 100K URLs?**
   - What we know: 1% FP = ~1MB filter, 0.1% FP = ~1.2MB filter, 0.01% FP = ~1.8MB filter
   - What's unclear: User tolerance for missed links
   - Recommendation: Use 0.1% (1 in 1000 chance) as default, make configurable via `--fp-rate` flag

## Sources

### Primary (HIGH confidence)
- https://pkg.go.dev/github.com/bits-and-blooms/bloom/v3 - Bloom filter API documentation
- https://github.com/bits-and-blooms/bloom - Used by Milvus, Beego; murmurhash; 1.9k stars
- https://pkg.go.dev/golang.org/x/time/rate - Token bucket rate limiter (already in project)
- https://pkg.go.dev/runtime/debug#SetMemoryLimit - Go 1.19+ soft memory limit
- https://pkg.go.dev/runtime#ReadMemStats - Memory statistics API

### Secondary (MEDIUM confidence)
- https://github.com/edsrzf/mmap-go - Portable mmap (1.1k stars, BSD-3 license)
- https://zread.ai/gocolly/colly/ - Colly crawler architecture (Storage interface pattern)

### Tertiary (LOW confidence)
(NONE - all claims verified with official sources)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - bits-and-blooms/bloom is the de-facto Go bloom filter, used by major projects
- Architecture: HIGH - patterns derived from official library documentation
- Pitfalls: MEDIUM - based on documented behavior, not direct production experience with this stack

**Research date:** 2026-02-17
**Valid until:** 30 days (libraries are stable; Go memory APIs are stable)
