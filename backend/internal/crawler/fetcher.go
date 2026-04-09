package crawler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// Fetcher retrieves web pages. Interface for testability.
type Fetcher interface {
	// Fetch retrieves the HTML content of the given URL.
	Fetch(ctx context.Context, url string) (string, error)
	// CheckRobotsTxt checks if the given path is allowed by robots.txt.
	CheckRobotsTxt(ctx context.Context, baseURL string, path string) (bool, error)
}

// HTTPFetcher implements Fetcher with rate limiting and timeouts.
type HTTPFetcher struct {
	client  *http.Client
	limiter *rate.Limiter
}

// NewHTTPFetcher creates a rate-limited HTTP fetcher (1 request/second).
func NewHTTPFetcher(timeout time.Duration) *HTTPFetcher {
	return &HTTPFetcher{
		client: &http.Client{
			Timeout: timeout,
		},
		// 1 request per second, burst of 1 (no bursting)
		limiter: rate.NewLimiter(rate.Every(1*time.Second), 1),
	}
}

func (f *HTTPFetcher) Fetch(ctx context.Context, targetURL string) (string, error) {
	// Wait for rate limiter
	if err := f.limiter.Wait(ctx); err != nil {
		return "", fmt.Errorf("rate limiter: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "HowMuchYouSay-Crawler/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := f.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching %s: %w", targetURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching %s: HTTP %d", targetURL, resp.StatusCode)
	}

	// Limit body read to 5MB to prevent memory issues
	body, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		return "", fmt.Errorf("reading response body: %w", err)
	}

	return string(body), nil
}

func (f *HTTPFetcher) CheckRobotsTxt(ctx context.Context, baseURL string, path string) (bool, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return false, fmt.Errorf("parsing base URL: %w", err)
	}

	robotsURL := fmt.Sprintf("%s://%s/robots.txt", parsed.Scheme, parsed.Host)

	// Robots.txt fetch skips the rate limiter (meta request, not a page crawl)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL, nil)
	if err != nil {
		return true, nil // Can't build request -> allow
	}
	req.Header.Set("User-Agent", "HowMuchYouSay-Crawler/1.0")

	resp, err := f.client.Do(req)
	if err != nil {
		return true, nil // Can't reach robots.txt -> allow
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return true, nil // No robots.txt -> everything allowed
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return true, nil
	}

	return parseRobotsTxt(string(body), path), nil
}

// parseRobotsTxt is a simple robots.txt parser for User-agent: * rules.
// Returns true if the path is allowed. Uses longest-match-wins semantics.
func parseRobotsTxt(content string, path string) bool {
	lines := strings.Split(content, "\n")
	inWildcardAgent := false

	// Track the most specific (longest) matching rule
	bestLen := -1
	allowed := true

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		lower := strings.ToLower(line)

		// Check for User-agent directive
		if strings.HasPrefix(lower, "user-agent:") {
			agent := strings.TrimSpace(strings.TrimPrefix(lower, "user-agent:"))
			inWildcardAgent = (agent == "*")
			continue
		}

		if !inWildcardAgent {
			continue
		}

		if strings.HasPrefix(lower, "disallow:") {
			disallowPath := strings.TrimSpace(line[len("Disallow:"):])
			if disallowPath != "" && strings.HasPrefix(path, disallowPath) && len(disallowPath) > bestLen {
				bestLen = len(disallowPath)
				allowed = false
			}
		}

		if strings.HasPrefix(lower, "allow:") {
			allowPath := strings.TrimSpace(line[len("Allow:"):])
			if allowPath != "" && strings.HasPrefix(path, allowPath) && len(allowPath) > bestLen {
				bestLen = len(allowPath)
				allowed = true
			}
		}
	}

	return allowed
}
