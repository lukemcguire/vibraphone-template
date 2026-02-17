package crawler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lukemcguire/zombiecrawl/result"
)

func TestDefaultRetryPolicy(t *testing.T) {
	policy := DefaultRetryPolicy()
	if policy.MaxRetries != 2 {
		t.Errorf("expected MaxRetries=2, got %d", policy.MaxRetries)
	}
	if policy.BaseDelay != 1*time.Second {
		t.Errorf("expected BaseDelay=1s, got %v", policy.BaseDelay)
	}
	if policy.MaxDelay != 30*time.Second {
		t.Errorf("expected MaxDelay=30s, got %v", policy.MaxDelay)
	}
}

func TestCheckURLWithRetry_SuccessOnFirstAttempt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := Config{
		RequestTimeout: 5 * time.Second,
		RetryPolicy:    DefaultRetryPolicy(),
	}
	job := CrawlJob{URL: server.URL, IsExternal: true}
	client := &http.Client{}

	res := CheckURLWithRetry(context.Background(), client, job, cfg, cfg.RetryPolicy)

	if res.Result != nil {
		t.Errorf("expected no error result, got %+v", res.Result)
	}
	if res.Err != nil {
		t.Errorf("expected no error, got %v", res.Err)
	}
}

func TestCheckURLWithRetry_RetriesOn5xx(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := atomic.AddInt32(&attempts, 1)
		if attempt < 3 {
			w.WriteHeader(http.StatusInternalServerError) // 500
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := Config{
		RequestTimeout: 5 * time.Second,
		RetryPolicy:    RetryPolicy{MaxRetries: 2, BaseDelay: 10 * time.Millisecond, MaxDelay: 100 * time.Millisecond},
	}
	job := CrawlJob{URL: server.URL, IsExternal: true}
	client := &http.Client{}

	res := CheckURLWithRetry(context.Background(), client, job, cfg, cfg.RetryPolicy)

	if res.Result != nil {
		t.Errorf("expected success after retries, got result: %+v", res.Result)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestCheckURLWithRetry_RetriesOn429(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := atomic.AddInt32(&attempts, 1)
		if attempt < 2 {
			w.WriteHeader(http.StatusTooManyRequests) // 429
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := Config{
		RequestTimeout: 5 * time.Second,
		RetryPolicy:    RetryPolicy{MaxRetries: 2, BaseDelay: 10 * time.Millisecond, MaxDelay: 100 * time.Millisecond},
	}
	job := CrawlJob{URL: server.URL, IsExternal: true}
	client := &http.Client{}

	res := CheckURLWithRetry(context.Background(), client, job, cfg, cfg.RetryPolicy)

	if res.Result != nil {
		t.Errorf("expected success after retries, got result: %+v", res.Result)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestCheckURLWithRetry_NoRetryOn4xx(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusNotFound) // 404
	}))
	defer server.Close()

	cfg := Config{
		RequestTimeout: 5 * time.Second,
		RetryPolicy:    RetryPolicy{MaxRetries: 2, BaseDelay: 10 * time.Millisecond, MaxDelay: 100 * time.Millisecond},
	}
	job := CrawlJob{URL: server.URL, IsExternal: true}
	client := &http.Client{}

	res := CheckURLWithRetry(context.Background(), client, job, cfg, cfg.RetryPolicy)

	if res.Result == nil {
		t.Error("expected broken link result for 404")
	} else if res.Result.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", res.Result.StatusCode)
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry on 404), got %d", attempts)
	}
}

func TestCheckURLWithRetry_ExhaustsRetries(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError) // Always 500
	}))
	defer server.Close()

	cfg := Config{
		RequestTimeout: 5 * time.Second,
		RetryPolicy:    RetryPolicy{MaxRetries: 2, BaseDelay: 10 * time.Millisecond, MaxDelay: 100 * time.Millisecond},
	}
	job := CrawlJob{URL: server.URL, IsExternal: true}
	client := &http.Client{}

	res := CheckURLWithRetry(context.Background(), client, job, cfg, cfg.RetryPolicy)

	if res.Result == nil {
		t.Error("expected broken link result")
	} else if res.Result.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", res.Result.StatusCode)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts (1 initial + 2 retries), got %d", attempts)
	}
}

func TestCheckURLWithRetry_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond) // Slow response
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := Config{
		RequestTimeout: 50 * time.Millisecond, // Short timeout
		RetryPolicy:    RetryPolicy{MaxRetries: 2, BaseDelay: 10 * time.Millisecond, MaxDelay: 100 * time.Millisecond},
	}
	job := CrawlJob{URL: server.URL, IsExternal: true}
	client := &http.Client{}

	// Use context that gets cancelled quickly
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	res := CheckURLWithRetry(ctx, client, job, cfg, cfg.RetryPolicy)

	// Should have a result (timeout or context cancelled)
	if res.Result == nil && res.Err == nil {
		t.Error("expected an error result due to context cancellation")
	}
}

func TestShouldRetry_NetworkErrors(t *testing.T) {
	tests := []struct {
		name       string
		result     CrawlResult
		shouldRetry bool
	}{
		{
			name: "timeout error",
			result: CrawlResult{
				Result: &result.LinkResult{
					StatusCode: 0,
					Error:      "context deadline exceeded",
				},
			},
			shouldRetry: true,
		},
		{
			name: "connection refused",
			result: CrawlResult{
				Result: &result.LinkResult{
					StatusCode: 0,
					Error:      "connection refused",
				},
			},
			shouldRetry: true,
		},
		{
			name: "DNS failure",
			result: CrawlResult{
				Result: &result.LinkResult{
					StatusCode: 0,
					Error:      "no such host",
				},
			},
			shouldRetry: true,
		},
		{
			name: "500 server error",
			result: CrawlResult{
				Result: &result.LinkResult{
					StatusCode: 500,
				},
			},
			shouldRetry: true,
		},
		{
			name: "429 rate limited",
			result: CrawlResult{
				Result: &result.LinkResult{
					StatusCode: 429,
				},
			},
			shouldRetry: true,
		},
		{
			name: "404 not found",
			result: CrawlResult{
				Result: &result.LinkResult{
					StatusCode: 404,
				},
			},
			shouldRetry: false,
		},
		{
			name: "403 forbidden",
			result: CrawlResult{
				Result: &result.LinkResult{
					StatusCode: 403,
				},
			},
			shouldRetry: false,
		},
		{
			name: "success",
			result: CrawlResult{
				Result: nil,
			},
			shouldRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldRetry(tt.result)
			if got != tt.shouldRetry {
				t.Errorf("shouldRetry() = %v, want %v", got, tt.shouldRetry)
			}
		})
	}
}
