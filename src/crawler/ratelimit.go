package crawler

import (
	"context"
	"math"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	// minRateFloor is the minimum rate in requests per second.
	// The adaptive limiter will never drop below this to avoid crawling too slowly.
	minRateFloor = 5.0

	// maxRateCeiling is the maximum rate in requests per second.
	// Prevents the limiter from becoming too aggressive.
	maxRateCeiling = 100.0

	// emaAlpha is the smoothing factor for Exponential Moving Average.
	// Lower values = more smoothing (slower to react to changes).
	// 0.2 means ~20% weight to new observation, ~80% to historical average.
	emaAlpha = 0.2

	// recoveryFactor is the multiplier for rate increase during recovery.
	// 1.1 = 10% increase per good RTT observation.
	recoveryFactor = 1.1

	// backoffFactor limits how much the rate can drop in a single step.
	// This prevents a single bad RTT from crashing the rate.
	backoffFactor = 0.5
)

// AdaptiveLimiter dynamically adjusts rate limiting based on server response times.
// It uses Exponential Moving Average (EMA) for RTT tracking to avoid single
// slow responses from skewing the rate too dramatically.
type AdaptiveLimiter struct {
	limiter   *rate.Limiter
	targetRTT time.Duration
	mu        sync.RWMutex

	// emaRTT is the exponential moving average of observed RTT values
	emaRTT time.Duration

	// currentRate is the current rate limit in requests per second (float for precision)
	currentRate float64

	// disabled indicates adaptive behavior is disabled (use fixed rate)
	disabled bool
}

// NewAdaptiveLimiter creates an adaptive rate limiter with the given initial rate
// and target RTT. The limiter will adjust its rate based on observed response times.
func NewAdaptiveLimiter(initialRPS int, targetRTT time.Duration) *AdaptiveLimiter {
	// Clamp initial rate to valid bounds
	clampedRPS := clampRateFloat(float64(initialRPS))

	return &AdaptiveLimiter{
		limiter:     rate.NewLimiter(rate.Limit(clampedRPS), int(clampedRPS)),
		targetRTT:   targetRTT,
		currentRate: clampedRPS,
		emaRTT:      targetRTT, // Initialize EMA at target
		disabled:    false,
	}
}

// Wait blocks until the rate limiter allows the next request or the context is cancelled.
// It is safe to call Wait from multiple goroutines concurrently.
func (a *AdaptiveLimiter) Wait(ctx context.Context) error {
	return a.limiter.Wait(ctx)
}

// ObserveRTT records a response time observation and adjusts the rate accordingly.
// This should be called after each HTTP request completes.
// The adjustment uses EMA to smooth out individual outliers.
func (a *AdaptiveLimiter) ObserveRTT(rtt time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.disabled {
		return
	}

	// Update EMA: new_ema = alpha * new_value + (1 - alpha) * old_ema
	newEMA := time.Duration(float64(emaAlpha)*float64(rtt) + (1-emaAlpha)*float64(a.emaRTT))
	a.emaRTT = newEMA

	// Calculate ratio of target to observed RTT
	// If observed RTT > target, ratio < 1 (slow down)
	// If observed RTT < target, ratio > 1 (can speed up)
	ratio := float64(a.targetRTT) / float64(newEMA)

	var newRate float64
	if ratio < 1 {
		// Server is slower than target - reduce rate, but limit how much per step
		proposedRate := a.currentRate * ratio
		// Don't drop more than backoffFactor (50%) in a single step
		minRate := a.currentRate * backoffFactor
		if proposedRate < minRate {
			newRate = minRate
		} else {
			newRate = proposedRate
		}
	} else {
		// Server is faster than target - increase rate gradually (10% per good RTT)
		newRate = a.currentRate * recoveryFactor
	}

	// Clamp to valid bounds
	newRate = clampRateFloat(newRate)

	// Update rate if changed significantly (more than 0.1 RPS)
	if math.Abs(newRate-a.currentRate) > 0.1 {
		a.currentRate = newRate
		a.limiter.SetLimit(rate.Limit(newRate))
		a.limiter.SetBurst(int(math.Ceil(newRate)))
	}
}

// SetRate manually overrides the current rate and disables adaptive behavior.
// Use this when the user explicitly sets a rate via CLI flag.
// The rate is clamped to the [minRateFloor, maxRateCeiling] range.
func (a *AdaptiveLimiter) SetRate(rps int) {
	a.mu.Lock()
	defer a.mu.Unlock()

	clamped := clampRateFloat(float64(rps))
	a.currentRate = clamped
	a.disabled = true // Manual override disables adaptation
	a.limiter.SetLimit(rate.Limit(clamped))
	a.limiter.SetBurst(int(math.Ceil(clamped)))
}

// CurrentRate returns the current rate limit in requests per second.
func (a *AdaptiveLimiter) CurrentRate() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return int(math.Round(a.currentRate))
}

// EnableAdaptation re-enables adaptive rate limiting after a manual override.
func (a *AdaptiveLimiter) EnableAdaptation() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.disabled = false
}

// clampRateFloat ensures the rate stays within valid bounds (float64 version).
func clampRateFloat(rps float64) float64 {
	if rps < minRateFloor {
		return minRateFloor
	}
	if rps > maxRateCeiling {
		return maxRateCeiling
	}
	return rps
}

// TargetRTT returns the configured target RTT for testing/debugging.
func (a *AdaptiveLimiter) TargetRTT() time.Duration {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.targetRTT
}

// CurrentEMA returns the current EMA of observed RTT values.
func (a *AdaptiveLimiter) CurrentEMA() time.Duration {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.emaRTT
}
