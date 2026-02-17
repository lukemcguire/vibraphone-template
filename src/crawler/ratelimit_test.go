package crawler

import (
	"context"
	"testing"
	"time"
)

func TestNewAdaptiveLimiter(t *testing.T) {
	tests := []struct {
		name       string
		initialRPS int
		targetRTT  time.Duration
		wantRate   int
	}{
		{
			name:       "default values",
			initialRPS: 10,
			targetRTT:  200 * time.Millisecond,
			wantRate:   10,
		},
		{
			name:       "high RPS",
			initialRPS: 50,
			targetRTT:  100 * time.Millisecond,
			wantRate:   50,
		},
		{
			name:       "low RPS",
			initialRPS: 5,
			targetRTT:  500 * time.Millisecond,
			wantRate:   5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter := NewAdaptiveLimiter(tt.initialRPS, tt.targetRTT)
			if limiter == nil {
				t.Fatal("NewAdaptiveLimiter returned nil")
			}
			if got := limiter.CurrentRate(); got != tt.wantRate {
				t.Errorf("CurrentRate() = %d, want %d", got, tt.wantRate)
			}
		})
	}
}

func TestAdaptiveLimiter_Wait(t *testing.T) {
	limiter := NewAdaptiveLimiter(10, 200*time.Millisecond)
	ctx := context.Background()

	// Wait should succeed immediately with no prior calls
	if err := limiter.Wait(ctx); err != nil {
		t.Errorf("Wait() failed: %v", err)
	}
}

func TestAdaptiveLimiter_Wait_ContextCancellation(t *testing.T) {
	limiter := NewAdaptiveLimiter(1, 200*time.Millisecond) // Very low rate
	ctx, cancel := context.WithCancel(context.Background())

	// First wait should succeed
	if err := limiter.Wait(ctx); err != nil {
		t.Fatalf("First Wait() failed: %v", err)
	}

	// Cancel context before second wait
	cancel()

	// Second wait should fail due to context cancellation
	err := limiter.Wait(ctx)
	if err == nil {
		t.Error("Wait() should have failed with cancelled context")
	}
}

func TestAdaptiveLimiter_ObserveRTT_Backoff(t *testing.T) {
	limiter := NewAdaptiveLimiter(10, 200*time.Millisecond)

	// Simulate slow responses - should back off
	for i := 0; i < 5; i++ {
		limiter.ObserveRTT(500 * time.Millisecond) // 2.5x target RTT
	}

	// Rate should decrease but stay above floor (5 RPS)
	got := limiter.CurrentRate()
	if got >= 10 {
		t.Errorf("CurrentRate() = %d, should have backed off below initial 10", got)
	}
	if got < 5 {
		t.Errorf("CurrentRate() = %d, should not drop below floor of 5", got)
	}
}

func TestAdaptiveLimiter_ObserveRTT_Recovery(t *testing.T) {
	limiter := NewAdaptiveLimiter(10, 200*time.Millisecond)

	// First, back off with slow responses
	for i := 0; i < 10; i++ {
		limiter.ObserveRTT(500 * time.Millisecond)
	}

	afterBackoff := limiter.CurrentRate()
	if afterBackoff >= 10 {
		t.Fatalf("Expected backoff, got rate %d", afterBackoff)
	}

	// Now simulate good responses - should recover (10% increase per good RTT)
	for i := 0; i < 20; i++ {
		limiter.ObserveRTT(100 * time.Millisecond) // 0.5x target RTT (good)
	}

	afterRecovery := limiter.CurrentRate()
	if afterRecovery <= afterBackoff {
		t.Errorf("CurrentRate() = %d, should have recovered above %d", afterRecovery, afterBackoff)
	}
}

func TestAdaptiveLimiter_ObserveRTT_MinimumFloor(t *testing.T) {
	limiter := NewAdaptiveLimiter(10, 200*time.Millisecond)

	// Simulate extremely slow responses - should hit floor
	for i := 0; i < 50; i++ {
		limiter.ObserveRTT(5 * time.Second)
	}

	got := limiter.CurrentRate()
	if got < 5 {
		t.Errorf("CurrentRate() = %d, minimum floor should be 5 RPS", got)
	}
}

func TestAdaptiveLimiter_ObserveRTT_MaximumCeiling(t *testing.T) {
	limiter := NewAdaptiveLimiter(10, 200*time.Millisecond)

	// Simulate extremely fast responses - should hit ceiling
	for i := 0; i < 50; i++ {
		limiter.ObserveRTT(1 * time.Millisecond)
	}

	got := limiter.CurrentRate()
	if got > 100 {
		t.Errorf("CurrentRate() = %d, maximum ceiling should be 100 RPS", got)
	}
}

func TestAdaptiveLimiter_SetRate(t *testing.T) {
	limiter := NewAdaptiveLimiter(10, 200*time.Millisecond)

	// Manual override
	limiter.SetRate(25)
	if got := limiter.CurrentRate(); got != 25 {
		t.Errorf("CurrentRate() = %d, want 25", got)
	}

	// Set to minimum floor
	limiter.SetRate(3) // Below floor
	if got := limiter.CurrentRate(); got != 5 {
		t.Errorf("CurrentRate() = %d, should be clamped to floor 5", got)
	}

	// Set to maximum ceiling
	limiter.SetRate(150) // Above ceiling
	if got := limiter.CurrentRate(); got != 100 {
		t.Errorf("CurrentRate() = %d, should be clamped to ceiling 100", got)
	}
}

func TestAdaptiveLimiter_EMA(t *testing.T) {
	// EMA should smooth out single outliers
	limiter := NewAdaptiveLimiter(10, 200*time.Millisecond)

	// Simulate steady state
	for i := 0; i < 10; i++ {
		limiter.ObserveRTT(200 * time.Millisecond) // At target
	}
	steadyRate := limiter.CurrentRate()

	// One outlier should not dramatically shift the rate
	limiter.ObserveRTT(5 * time.Second) // One very slow request

	afterOutlier := limiter.CurrentRate()

	// The rate should drop, but not as dramatically as without EMA
	// (without EMA, this would be a 25x ratio, causing rate to drop to floor)
	// With EMA, the drop should be more gradual
	if afterOutlier >= steadyRate {
		t.Errorf("Rate should drop after slow RTT, got %d (was %d)", afterOutlier, steadyRate)
	}

	// The drop should not be catastrophic (more than 50% in one step)
	dropRatio := float64(steadyRate-afterOutlier) / float64(steadyRate)
	if dropRatio > 0.5 {
		t.Errorf("EMA should smooth outliers, but rate dropped %.1f%% (from %d to %d)",
			dropRatio*100, steadyRate, afterOutlier)
	}
}

func TestAdaptiveLimiter_ConcurrentAccess(t *testing.T) {
	limiter := NewAdaptiveLimiter(100, 200*time.Millisecond) // High rate for fast test
	ctx := context.Background()

	// Simulate concurrent usage
	done := make(chan bool)

	for range 10 {
		go func() {
			for range 20 {
				_ = limiter.Wait(ctx)
				limiter.ObserveRTT(time.Duration(100) * time.Millisecond)
				_ = limiter.CurrentRate()
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for range 10 {
		<-done
	}

	// If we get here without race conditions, the test passes
}

func TestAdaptiveLimiter_EnableAdaptation(t *testing.T) {
	limiter := NewAdaptiveLimiter(10, 200*time.Millisecond)

	// Simulate some observations to get a known state
	limiter.ObserveRTT(300 * time.Millisecond)
	afterObs := limiter.CurrentRate()

	// SetRate disables adaptation
	limiter.SetRate(50)
	if got := limiter.CurrentRate(); got != 50 {
		t.Fatalf("SetRate(50) failed, got %d", got)
	}

	// Observe RTT while disabled - should not change rate
	limiter.ObserveRTT(5000 * time.Millisecond)
	if got := limiter.CurrentRate(); got != 50 {
		t.Errorf("Rate changed while adaptation disabled: got %d, want 50", got)
	}

	// Re-enable adaptation
	limiter.EnableAdaptation()

	// Now observe RTT should affect rate again
	limiter.ObserveRTT(500 * time.Millisecond)
	newRate := limiter.CurrentRate()
	if newRate == 50 {
		t.Errorf("Rate did not change after EnableAdaptation, still at 50")
	}
	if newRate > afterObs {
		t.Logf("Rate after re-enabling: %d (was %d before disable)", newRate, afterObs)
	}
}

func TestAdaptiveLimiter_TargetRTT(t *testing.T) {
	targetRTT := 150 * time.Millisecond
	limiter := NewAdaptiveLimiter(10, targetRTT)

	if got := limiter.TargetRTT(); got != targetRTT {
		t.Errorf("TargetRTT() = %v, want %v", got, targetRTT)
	}
}

func TestAdaptiveLimiter_CurrentEMA(t *testing.T) {
	targetRTT := 200 * time.Millisecond
	limiter := NewAdaptiveLimiter(10, targetRTT)

	// EMA should start at target RTT
	if got := limiter.CurrentEMA(); got != targetRTT {
		t.Errorf("Initial CurrentEMA() = %v, want %v", got, targetRTT)
	}

	// Observe some RTT values
	limiter.ObserveRTT(300 * time.Millisecond)
	limiter.ObserveRTT(300 * time.Millisecond)
	limiter.ObserveRTT(300 * time.Millisecond)

	// EMA should now be closer to 300ms than the initial 200ms
	ema := limiter.CurrentEMA()
	if ema <= targetRTT {
		t.Errorf("CurrentEMA() = %v, should have moved toward 300ms observations", ema)
	}
	if ema > 300*time.Millisecond {
		t.Errorf("CurrentEMA() = %v, should not exceed observed values significantly", ema)
	}
}
