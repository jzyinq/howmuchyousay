package crawler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPFetcher_Fetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body>Hello</body></html>"))
	}))
	defer server.Close()

	fetcher := crawler.NewHTTPFetcher(5 * time.Second)
	ctx := context.Background()

	body, err := fetcher.Fetch(ctx, server.URL)
	require.NoError(t, err)
	assert.Contains(t, body, "<html>")
	assert.Contains(t, body, "Hello")
}

func TestHTTPFetcher_FetchReturnsErrorOnBadStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	fetcher := crawler.NewHTTPFetcher(5 * time.Second)
	ctx := context.Background()

	_, err := fetcher.Fetch(ctx, server.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestHTTPFetcher_FetchRespectsContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Write([]byte("slow"))
	}))
	defer server.Close()

	fetcher := crawler.NewHTTPFetcher(5 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := fetcher.Fetch(ctx, server.URL)
	require.Error(t, err)
}

func TestHTTPFetcher_RateLimiting(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	fetcher := crawler.NewHTTPFetcher(5 * time.Second)
	ctx := context.Background()

	start := time.Now()
	for i := 0; i < 3; i++ {
		_, err := fetcher.Fetch(ctx, server.URL)
		require.NoError(t, err)
	}
	elapsed := time.Since(start)

	// 3 requests at 1/sec rate limit: first fires immediately, 2nd waits ~1s, 3rd waits ~1s
	// So minimum ~2 seconds for 3 requests.
	assert.GreaterOrEqual(t, elapsed.Seconds(), 1.5, "Rate limiting should enforce ~1 req/sec")
	assert.Equal(t, 3, requestCount)
}

func TestHTTPFetcher_CheckRobotsTxt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.Write([]byte("User-agent: *\nDisallow: /private/\nAllow: /"))
			return
		}
		w.Write([]byte("page"))
	}))
	defer server.Close()

	fetcher := crawler.NewHTTPFetcher(5 * time.Second)
	ctx := context.Background()

	allowed, err := fetcher.CheckRobotsTxt(ctx, server.URL, "/products")
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = fetcher.CheckRobotsTxt(ctx, server.URL, "/private/secret")
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestHTTPFetcher_CheckRobotsTxt_NoFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte("page"))
	}))
	defer server.Close()

	fetcher := crawler.NewHTTPFetcher(5 * time.Second)
	ctx := context.Background()

	// No robots.txt -> everything allowed
	allowed, err := fetcher.CheckRobotsTxt(ctx, server.URL, "/anything")
	require.NoError(t, err)
	assert.True(t, allowed)
}
