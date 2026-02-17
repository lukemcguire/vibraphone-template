package crawler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewRobotsChecker_InitializesDefaults(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}
	checker := NewRobotsChecker(client)

	if checker == nil {
		t.Fatal("NewRobotsChecker returned nil")
	}
	if checker.client != client {
		t.Error("client not wired correctly")
	}
	if checker.cacheTTL != time.Hour {
		t.Errorf("cacheTTL = %v, want %v", checker.cacheTTL, time.Hour)
	}
}

func TestRobotsChecker_Allowed(t *testing.T) {
	testCases := []struct {
		name       string
		robotsTxt  string
		statusCode int
		url        string
		userAgent  string
		want       bool
	}{
		{
			name: "disallow specific path",
			robotsTxt: `User-agent: *
Disallow: /private/`,
			statusCode: http.StatusOK,
			url:        "http://example.com/private/secret",
			userAgent:  "testbot",
			want:       false,
		},
		{
			name: "allow public path",
			robotsTxt: `User-agent: *
Disallow: /private/`,
			statusCode: http.StatusOK,
			url:        "http://example.com/public/page",
			userAgent:  "testbot",
			want:       true,
		},
		{
			name:       "404 allows all",
			robotsTxt:  "",
			statusCode: http.StatusNotFound,
			url:        "http://example.com/any/path",
			userAgent:  "testbot",
			want:       true,
		},
		{
			name:       "500 allows all",
			robotsTxt:  "",
			statusCode: http.StatusInternalServerError,
			url:        "http://example.com/any/path",
			userAgent:  "testbot",
			want:       true,
		},
		{
			name:       "empty robots.txt allows all",
			robotsTxt:  "",
			statusCode: http.StatusOK,
			url:        "http://example.com/any/path",
			userAgent:  "testbot",
			want:       true,
		},
		{
			name: "specific user agent disallowed",
			robotsTxt: `User-agent: EvilBot
Disallow: /`,
			statusCode: http.StatusOK,
			url:        "http://example.com/page",
			userAgent:  "EvilBot",
			want:       false,
		},
		{
			name: "other user agent allowed",
			robotsTxt: `User-agent: EvilBot
Disallow: /`,
			statusCode: http.StatusOK,
			url:        "http://example.com/page",
			userAgent:  "GoodBot",
			want:       true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(respWriter http.ResponseWriter, req *http.Request) {
				if req.URL.Path == "/robots.txt" {
					respWriter.WriteHeader(testCase.statusCode)
					if testCase.statusCode == http.StatusOK && testCase.robotsTxt != "" {
						if _, err := respWriter.Write([]byte(testCase.robotsTxt)); err != nil {
							t.Errorf("write robots.txt: %v", err)
						}
					}
					return
				}
				respWriter.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			// Replace host in URL with test server
			client := &http.Client{Timeout: 5 * time.Second}
			checker := NewRobotsChecker(client)

			// Construct URL with test server's host
			targetURL := server.URL + "/any/path"
			if testCase.url != "" {
				// Extract path from original URL
				targetURL = server.URL + "/private/secret"
				if testCase.want {
					targetURL = server.URL + "/public/page"
				}
				if testCase.name == "specific user agent disallowed" || testCase.name == "other user agent allowed" {
					targetURL = server.URL + "/page"
				}
			}

			got, err := checker.Allowed(context.Background(), targetURL, testCase.userAgent)
			if err != nil && testCase.want {
				t.Errorf("Allowed() error = %v, want nil", err)
			}
			if got != testCase.want {
				t.Errorf("Allowed() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestRobotsChecker_CacheExpiration(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(respWriter http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/robots.txt" {
			requestCount++
			respWriter.WriteHeader(http.StatusOK)
			if _, err := respWriter.Write([]byte(`User-agent: *
Disallow: /blocked/`)); err != nil {
				t.Errorf("write robots.txt: %v", err)
			}
			return
		}
		respWriter.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	checker := NewRobotsChecker(client)
	// Override TTL for testing
	checker.cacheTTL = 100 * time.Millisecond

	// First request - should fetch
	allowed1, err1 := checker.Allowed(context.Background(), server.URL+"/blocked/page", "testbot")
	if err1 != nil {
		t.Errorf("First request error: %v", err1)
	}
	if allowed1 {
		t.Error("First request should be disallowed")
	}
	if requestCount != 1 {
		t.Errorf("Expected 1 request, got %d", requestCount)
	}

	// Second request - should use cache
	allowed2, err2 := checker.Allowed(context.Background(), server.URL+"/blocked/page2", "testbot")
	if err2 != nil {
		t.Errorf("Second request error: %v", err2)
	}
	if allowed2 {
		t.Error("Second request should be disallowed (from cache)")
	}
	if requestCount != 1 {
		t.Errorf("Expected 1 request (cached), got %d", requestCount)
	}

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

	// Third request - should fetch again
	allowed3, err3 := checker.Allowed(context.Background(), server.URL+"/blocked/page3", "testbot")
	if err3 != nil {
		t.Errorf("Third request error: %v", err3)
	}
	if allowed3 {
		t.Error("Third request should be disallowed")
	}
	if requestCount != 2 {
		t.Errorf("Expected 2 requests (cache expired), got %d", requestCount)
	}
}

func TestRobotsChecker_TimeoutAllowsAll(t *testing.T) {
	// Server that never responds
	server := httptest.NewServer(http.HandlerFunc(func(respWriter http.ResponseWriter, req *http.Request) {
		time.Sleep(10 * time.Second)
		respWriter.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Very short timeout client
	client := &http.Client{Timeout: 10 * time.Millisecond}
	checker := NewRobotsChecker(client)

	allowed, err := checker.Allowed(context.Background(), server.URL+"/any/path", "testbot")
	// Timeout returns an error but still allows crawling (fail-open)
	if !allowed {
		t.Error("Timeout should allow all")
	}
	if err == nil {
		t.Error("Timeout should return an error for visibility")
	}
}

func TestRobotsChecker_ClearCache(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(respWriter http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/robots.txt" {
			requestCount++
			respWriter.WriteHeader(http.StatusOK)
			if _, err := respWriter.Write([]byte(`User-agent: *
Disallow: /blocked/`)); err != nil {
				t.Errorf("write robots.txt: %v", err)
			}
			return
		}
		respWriter.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	checker := NewRobotsChecker(client)

	// First request
	_, err1 := checker.Allowed(context.Background(), server.URL+"/blocked/page", "testbot")
	if err1 != nil {
		t.Errorf("First request error: %v", err1)
	}
	if requestCount != 1 {
		t.Errorf("Expected 1 request, got %d", requestCount)
	}

	// Clear cache
	checker.ClearCache()

	// Should fetch again
	_, err2 := checker.Allowed(context.Background(), server.URL+"/blocked/page", "testbot")
	if err2 != nil {
		t.Errorf("Second request error: %v", err2)
	}
	if requestCount != 2 {
		t.Errorf("Expected 2 requests after ClearCache, got %d", requestCount)
	}
}
