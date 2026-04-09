# Plan 2: Crawler CLI & AI Agent

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the product crawling system: HTTP fetcher with rate limiting, structured data extractor (JSON-LD/microdata), OpenAI-powered AI agent for HTML analysis, product validator, per-crawl file logger, and a standalone CLI binary for manual crawling.

**Architecture:** The crawler package (`internal/crawler/`) is a self-contained component used by both the CLI binary and the game server. All external dependencies (HTTP client, AI API) are behind interfaces for testability. The crawler orchestrator ties together: fetching pages, extracting structured data, falling back to AI extraction, validating products, and persisting results via the store layer (from Plan 1). Tests use `httptest` servers and mock AI clients - no real network calls.

**Tech Stack:** Go 1.23+, `net/http` + `httptest`, `encoding/json`, `github.com/openai/openai-go/v3` (official OpenAI Go SDK), `golang.org/x/time/rate` (rate limiter), stores from Plan 1

**Plan sequence:** This is Plan 2 of 5:
1. Database, Models & Store Layer (done)
2. **Crawler CLI & AI Agent** (this plan)
3. Game Engine
4. Single Player (API + Frontend)
5. Multiplayer (WebSocket + Frontend)

**Prerequisite:** Plan 1 must be fully implemented (all stores, models, migrations, config).

---

## File Structure

```
backend/
├── cmd/
│   └── crawler/
│       └── main.go              # CLI entry point: flags, DB connect, run crawler
├── internal/
│   └── crawler/
│       ├── fetcher.go           # Fetcher interface + HTTPFetcher (rate-limited HTTP client)
│       ├── fetcher_test.go      # Tests with httptest server
│       ├── extractor.go         # Extract products from HTML (JSON-LD, microdata, OG)
│       ├── extractor_test.go    # Unit tests with HTML fixtures
│       ├── validator.go         # Validate + deduplicate extracted products
│       ├── validator_test.go    # Unit tests
│       ├── ai_client.go         # AIClient interface + OpenAIClient implementation
│       ├── ai_client_test.go    # Tests with mock AI (httptest for OpenAI API)
│       ├── logger.go            # CrawlLogger: per-crawl file logging
│       ├── logger_test.go       # Tests with temp dirs
│       ├── crawler.go           # Crawler orchestrator: ties all components together
│       └── crawler_test.go      # Integration tests with mock fetcher + mock AI
└── go.mod                       # (already exists from Plan 1)
```

Each file has a single responsibility:
- **fetcher.go** - HTTP requests with rate limiting (1 req/s) and robots.txt checking
- **extractor.go** - Parse HTML for structured product data (JSON-LD `@type: Product`, Open Graph). Note: microdata is mentioned in the spec but skipped here (YAGNI) - JSON-LD + OG covers 95%+ of real shops. Can be added later if needed.
- **validator.go** - Validate extracted products (price > 0, name not empty, URL valid) + deduplicate by normalized name
- **ai_client.go** - Interface for AI product extraction + OpenAI implementation using official `openai-go` SDK
- **logger.go** - Structured per-crawl file logging with log levels (FETCH, PARSE, AI_REQUEST, AI_RESPONSE, NAVIGATE, PRODUCT_FOUND, VALIDATION, ERROR)
- **crawler.go** - Main orchestrator: runs the full crawl pipeline, coordinates all components, reports progress via callback

**Note on cache/fallback:** The spec's "cache fallback" logic (use existing products if crawl fails) is a **game server concern** (Plan 3/4), not a crawler concern. The crawler just crawls and persists. The game server decides what to do when a crawl returns too few products.

---

### Task 1: Product Data Type & Crawler Config

**Files:**
- Create: `backend/internal/crawler/crawler.go` (initial: just types)

This task defines the shared data types used across all crawler files. We put them in `crawler.go` since it's the main orchestrator file and will grow later.

- [ ] **Step 1: Create crawler.go with shared types**

Create `backend/internal/crawler/crawler.go`:

```go
package crawler

import "time"

// RawProduct represents a product extracted from a web page before validation.
// This is the intermediate format used by extractors and AI before persisting.
type RawProduct struct {
	Name      string  `json:"name"`
	Price     float64 `json:"price"`
	ImageURL  string  `json:"image_url"`
	SourceURL string  `json:"source_url"`
}

// CrawlConfig holds settings for a single crawl run.
type CrawlConfig struct {
	// URL is the shop URL to crawl.
	URL string
	// Timeout is the maximum duration for the entire crawl.
	Timeout time.Duration
	// MinProducts is the minimum number of products to collect.
	MinProducts int
	// Verbose enables extra stdout output (for CLI mode).
	Verbose bool
}

// DefaultCrawlConfig returns config with sensible defaults.
func DefaultCrawlConfig(url string) CrawlConfig {
	return CrawlConfig{
		URL:         url,
		Timeout:     90 * time.Second,
		MinProducts: 20,
		Verbose:     false,
	}
}

// ProgressFunc is called during crawling to report progress.
// Used by CLI for stdout output and by server for WebSocket updates.
type ProgressFunc func(event string, message string)
```

- [ ] **Step 2: Verify it compiles**

Run: `cd backend && go build ./internal/crawler/`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "feat(crawler): add RawProduct type and CrawlConfig"
```

---

### Task 2: Fetcher (HTTP Client with Rate Limiting)

**Files:**
- Create: `backend/internal/crawler/fetcher.go`
- Create: `backend/internal/crawler/fetcher_test.go`

The Fetcher wraps HTTP requests with: rate limiting (1 req/s), configurable timeout, User-Agent header, and robots.txt checking.

- [ ] **Step 1: Write failing tests for Fetcher**

Create `backend/internal/crawler/fetcher_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/crawler/ -v -run TestHTTPFetcher -count=1`
Expected: FAIL - `crawler.NewHTTPFetcher` undefined.

- [ ] **Step 3: Install rate limiter dependency**

```bash
cd backend && go get golang.org/x/time/rate
```

- [ ] **Step 4: Implement Fetcher**

Create `backend/internal/crawler/fetcher.go`:

```go
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
// Returns true if the path is allowed.
func parseRobotsTxt(content string, path string) bool {
	lines := strings.Split(content, "\n")
	inWildcardAgent := false
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
			if disallowPath != "" && strings.HasPrefix(path, disallowPath) {
				allowed = false
			}
		}

		if strings.HasPrefix(lower, "allow:") {
			allowPath := strings.TrimSpace(line[len("Allow:"):])
			if allowPath != "" && strings.HasPrefix(path, allowPath) {
				allowed = true
			}
		}
	}

	return allowed
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd backend && go test ./internal/crawler/ -v -run TestHTTPFetcher -count=1`
Expected: All 5 tests PASS.

Note: `TestHTTPFetcher_RateLimiting` takes ~2 seconds due to the rate limiter.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(crawler): add HTTP fetcher with rate limiting and robots.txt support"
```

---

### Task 3: Extractor (Structured Data from HTML)

**Files:**
- Create: `backend/internal/crawler/extractor.go`
- Create: `backend/internal/crawler/extractor_test.go`

The extractor parses HTML to find product data from structured formats: JSON-LD (`@type: Product`), Open Graph meta tags, and basic microdata. No AI needed for these - they're standard formats.

- [ ] **Step 1: Write failing tests for Extractor**

Create `backend/internal/crawler/extractor_test.go`:

```go
package crawler_test

import (
	"testing"

	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractProducts_JSONLD_SingleProduct(t *testing.T) {
	html := `<html><head>
	<script type="application/ld+json">
	{
		"@context": "https://schema.org",
		"@type": "Product",
		"name": "iPhone 15 Pro",
		"image": "https://example.com/iphone.jpg",
		"offers": {
			"@type": "Offer",
			"price": "5499.00",
			"priceCurrency": "PLN"
		}
	}
	</script>
	</head><body></body></html>`

	products := crawler.ExtractProducts(html, "https://example.com/iphone")
	require.Len(t, products, 1)
	assert.Equal(t, "iPhone 15 Pro", products[0].Name)
	assert.Equal(t, 5499.00, products[0].Price)
	assert.Equal(t, "https://example.com/iphone.jpg", products[0].ImageURL)
	assert.Equal(t, "https://example.com/iphone", products[0].SourceURL)
}

func TestExtractProducts_JSONLD_MultipleProducts(t *testing.T) {
	html := `<html><head>
	<script type="application/ld+json">
	[
		{
			"@context": "https://schema.org",
			"@type": "Product",
			"name": "Product A",
			"offers": {"price": "100.00"}
		},
		{
			"@context": "https://schema.org",
			"@type": "Product",
			"name": "Product B",
			"offers": {"price": "200.00"}
		}
	]
	</script>
	</head><body></body></html>`

	products := crawler.ExtractProducts(html, "https://example.com/list")
	require.Len(t, products, 2)
	assert.Equal(t, "Product A", products[0].Name)
	assert.Equal(t, "Product B", products[1].Name)
}

func TestExtractProducts_JSONLD_OffersArray(t *testing.T) {
	html := `<html><head>
	<script type="application/ld+json">
	{
		"@type": "Product",
		"name": "Multi-offer Product",
		"offers": [
			{"price": "99.99"},
			{"price": "109.99"}
		]
	}
	</script>
	</head><body></body></html>`

	products := crawler.ExtractProducts(html, "https://example.com/multi")
	require.Len(t, products, 1)
	assert.Equal(t, "Multi-offer Product", products[0].Name)
	// Takes first offer price
	assert.Equal(t, 99.99, products[0].Price)
}

func TestExtractProducts_JSONLD_ImageArray(t *testing.T) {
	html := `<html><head>
	<script type="application/ld+json">
	{
		"@type": "Product",
		"name": "Image Array Product",
		"image": ["https://example.com/img1.jpg", "https://example.com/img2.jpg"],
		"offers": {"price": "50.00"}
	}
	</script>
	</head><body></body></html>`

	products := crawler.ExtractProducts(html, "https://example.com/images")
	require.Len(t, products, 1)
	assert.Equal(t, "https://example.com/img1.jpg", products[0].ImageURL)
}

func TestExtractProducts_JSONLD_NestedGraph(t *testing.T) {
	html := `<html><head>
	<script type="application/ld+json">
	{
		"@context": "https://schema.org",
		"@graph": [
			{
				"@type": "WebPage",
				"name": "Some page"
			},
			{
				"@type": "Product",
				"name": "Nested Product",
				"offers": {"price": "299.99"}
			}
		]
	}
	</script>
	</head><body></body></html>`

	products := crawler.ExtractProducts(html, "https://example.com/nested")
	require.Len(t, products, 1)
	assert.Equal(t, "Nested Product", products[0].Name)
	assert.Equal(t, 299.99, products[0].Price)
}

func TestExtractProducts_OpenGraph(t *testing.T) {
	html := `<html><head>
	<meta property="og:type" content="product" />
	<meta property="og:title" content="OG Product" />
	<meta property="product:price:amount" content="199.99" />
	<meta property="og:image" content="https://example.com/og.jpg" />
	</head><body></body></html>`

	products := crawler.ExtractProducts(html, "https://example.com/og")
	require.Len(t, products, 1)
	assert.Equal(t, "OG Product", products[0].Name)
	assert.Equal(t, 199.99, products[0].Price)
	assert.Equal(t, "https://example.com/og.jpg", products[0].ImageURL)
}

func TestExtractProducts_NoStructuredData(t *testing.T) {
	html := `<html><head><title>Just a page</title></head>
	<body><h1>No products here</h1></body></html>`

	products := crawler.ExtractProducts(html, "https://example.com/empty")
	assert.Empty(t, products)
}

func TestExtractProducts_InvalidJSON(t *testing.T) {
	html := `<html><head>
	<script type="application/ld+json">{invalid json}</script>
	</head><body></body></html>`

	products := crawler.ExtractProducts(html, "https://example.com/broken")
	assert.Empty(t, products)
}

func TestExtractLinks(t *testing.T) {
	html := `<html><body>
	<a href="/products/1">Product 1</a>
	<a href="/products/2">Product 2</a>
	<a href="/category/electronics">Electronics</a>
	<a href="https://external.com/page">External</a>
	<a href="#">Empty</a>
	</body></html>`

	links := crawler.ExtractLinks(html, "https://example.com")
	assert.Contains(t, links, "https://example.com/products/1")
	assert.Contains(t, links, "https://example.com/products/2")
	assert.Contains(t, links, "https://example.com/category/electronics")
	// External links included
	assert.Contains(t, links, "https://external.com/page")
	// Fragment-only links excluded
	assert.NotContains(t, links, "#")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/crawler/ -v -run "TestExtract" -count=1`
Expected: FAIL - `crawler.ExtractProducts` undefined.

- [ ] **Step 3: Implement Extractor**

Create `backend/internal/crawler/extractor.go`:

```go
package crawler

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// ExtractProducts extracts product data from HTML using structured data formats.
// Tries JSON-LD first, then Open Graph. Returns all products found.
func ExtractProducts(html string, sourceURL string) []RawProduct {
	var products []RawProduct

	// Try JSON-LD first (highest quality data)
	products = append(products, extractJSONLD(html, sourceURL)...)

	// If JSON-LD found products, return them
	if len(products) > 0 {
		return products
	}

	// Try Open Graph as fallback
	products = append(products, extractOpenGraph(html, sourceURL)...)

	return products
}

// ExtractLinks extracts all <a href="..."> links from HTML, resolving relative URLs.
func ExtractLinks(html string, baseURL string) []string {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	// Simple regex for <a href="..."> - good enough for crawling
	re := regexp.MustCompile(`<a\s+[^>]*href\s*=\s*["']([^"']+)["']`)
	matches := re.FindAllStringSubmatch(html, -1)

	seen := make(map[string]bool)
	var links []string

	for _, match := range matches {
		href := strings.TrimSpace(match[1])

		// Skip empty, fragment-only, javascript, mailto links
		if href == "" || href == "#" || strings.HasPrefix(href, "#") ||
			strings.HasPrefix(href, "javascript:") || strings.HasPrefix(href, "mailto:") {
			continue
		}

		// Resolve relative URLs
		resolved, err := base.Parse(href)
		if err != nil {
			continue
		}

		fullURL := resolved.String()
		if !seen[fullURL] {
			seen[fullURL] = true
			links = append(links, fullURL)
		}
	}

	return links
}

// --- JSON-LD extraction ---

func extractJSONLD(html string, sourceURL string) []RawProduct {
	// Find all <script type="application/ld+json"> blocks
	re := regexp.MustCompile(`(?s)<script[^>]*type\s*=\s*["']application/ld\+json["'][^>]*>(.*?)</script>`)
	matches := re.FindAllStringSubmatch(html, -1)

	var products []RawProduct

	for _, match := range matches {
		jsonStr := strings.TrimSpace(match[1])
		products = append(products, parseJSONLDBlock(jsonStr, sourceURL)...)
	}

	return products
}

func parseJSONLDBlock(jsonStr string, sourceURL string) []RawProduct {
	// Try parsing as a single object first
	var single map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &single); err == nil {
		return extractProductsFromJSONLDObject(single, sourceURL)
	}

	// Try parsing as an array of objects
	var array []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &array); err == nil {
		var products []RawProduct
		for _, obj := range array {
			products = append(products, extractProductsFromJSONLDObject(obj, sourceURL)...)
		}
		return products
	}

	return nil
}

func extractProductsFromJSONLDObject(obj map[string]interface{}, sourceURL string) []RawProduct {
	var products []RawProduct

	// Check for @graph (nested structure)
	if graph, ok := obj["@graph"]; ok {
		if graphArray, ok := graph.([]interface{}); ok {
			for _, item := range graphArray {
				if itemMap, ok := item.(map[string]interface{}); ok {
					products = append(products, extractProductsFromJSONLDObject(itemMap, sourceURL)...)
				}
			}
			return products
		}
	}

	// Check if this object is a Product
	typeVal, _ := obj["@type"].(string)
	if !strings.EqualFold(typeVal, "Product") {
		return nil
	}

	name, _ := obj["name"].(string)
	if name == "" {
		return nil
	}

	price := extractPriceFromOffers(obj)
	imageURL := extractImageFromJSONLD(obj)

	if price > 0 {
		products = append(products, RawProduct{
			Name:      name,
			Price:     price,
			ImageURL:  imageURL,
			SourceURL: sourceURL,
		})
	}

	return products
}

func extractPriceFromOffers(obj map[string]interface{}) float64 {
	offers, ok := obj["offers"]
	if !ok {
		return 0
	}

	// Offers can be a single object or an array
	switch v := offers.(type) {
	case map[string]interface{}:
		return parsePriceValue(v["price"])
	case []interface{}:
		if len(v) > 0 {
			if firstOffer, ok := v[0].(map[string]interface{}); ok {
				return parsePriceValue(firstOffer["price"])
			}
		}
	}

	return 0
}

func parsePriceValue(v interface{}) float64 {
	switch p := v.(type) {
	case float64:
		return p
	case string:
		// Clean common price formatting
		cleaned := strings.ReplaceAll(p, ",", ".")
		cleaned = strings.ReplaceAll(cleaned, " ", "")
		price, err := strconv.ParseFloat(cleaned, 64)
		if err != nil {
			return 0
		}
		return price
	}
	return 0
}

func extractImageFromJSONLD(obj map[string]interface{}) string {
	img, ok := obj["image"]
	if !ok {
		return ""
	}

	switch v := img.(type) {
	case string:
		return v
	case []interface{}:
		if len(v) > 0 {
			if s, ok := v[0].(string); ok {
				return s
			}
		}
	}

	return ""
}

// --- Open Graph extraction ---

func extractOpenGraph(html string, sourceURL string) []RawProduct {
	metas := extractMetaTags(html)

	ogType := metas["og:type"]
	if !strings.EqualFold(ogType, "product") {
		return nil
	}

	name := metas["og:title"]
	if name == "" {
		return nil
	}

	priceStr := metas["product:price:amount"]
	if priceStr == "" {
		return nil
	}

	price := parsePriceValue(priceStr)
	if price <= 0 {
		return nil
	}

	imageURL := metas["og:image"]

	return []RawProduct{{
		Name:      name,
		Price:     price,
		ImageURL:  imageURL,
		SourceURL: sourceURL,
	}}
}

// extractMetaTags extracts <meta property="..." content="..."> tags.
func extractMetaTags(html string) map[string]string {
	re := regexp.MustCompile(`<meta\s+[^>]*property\s*=\s*["']([^"']+)["'][^>]*content\s*=\s*["']([^"']+)["'][^>]*/?>`)
	matches := re.FindAllStringSubmatch(html, -1)

	result := make(map[string]string)
	for _, match := range matches {
		result[match[1]] = match[2]
	}

	// Also try the reverse order: content before property
	re2 := regexp.MustCompile(`<meta\s+[^>]*content\s*=\s*["']([^"']+)["'][^>]*property\s*=\s*["']([^"']+)["'][^>]*/?>`)
	matches2 := re2.FindAllStringSubmatch(html, -1)
	for _, match := range matches2 {
		if _, exists := result[match[2]]; !exists {
			result[match[2]] = match[1]
		}
	}

	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/crawler/ -v -run "TestExtract" -count=1`
Expected: All 9 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat(crawler): add structured data extractor (JSON-LD, Open Graph, links)"
```

---

### Task 4: Validator (Product Validation & Deduplication)

**Files:**
- Create: `backend/internal/crawler/validator.go`
- Create: `backend/internal/crawler/validator_test.go`

Validates extracted products: price > 0, name not empty, URL validation for images. Deduplicates by normalized name.

- [ ] **Step 1: Write failing tests for Validator**

Create `backend/internal/crawler/validator_test.go`:

```go
package crawler_test

import (
	"testing"

	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateProducts_Valid(t *testing.T) {
	raw := []crawler.RawProduct{
		{Name: "Laptop Dell XPS", Price: 5999.99, ImageURL: "https://img.com/dell.jpg", SourceURL: "https://shop.com/dell"},
		{Name: "iPhone 15", Price: 4999.00, ImageURL: "https://img.com/iphone.jpg", SourceURL: "https://shop.com/iphone"},
	}

	valid, rejected := crawler.ValidateProducts(raw)
	assert.Len(t, valid, 2)
	assert.Empty(t, rejected)
}

func TestValidateProducts_EmptyName(t *testing.T) {
	raw := []crawler.RawProduct{
		{Name: "", Price: 100.00, ImageURL: "", SourceURL: ""},
	}

	valid, rejected := crawler.ValidateProducts(raw)
	assert.Empty(t, valid)
	require.Len(t, rejected, 1)
	assert.Contains(t, rejected[0].Reason, "empty name")
}

func TestValidateProducts_ZeroPrice(t *testing.T) {
	raw := []crawler.RawProduct{
		{Name: "Free Product", Price: 0, ImageURL: "", SourceURL: ""},
	}

	valid, rejected := crawler.ValidateProducts(raw)
	assert.Empty(t, valid)
	require.Len(t, rejected, 1)
	assert.Contains(t, rejected[0].Reason, "price")
}

func TestValidateProducts_NegativePrice(t *testing.T) {
	raw := []crawler.RawProduct{
		{Name: "Negative Price", Price: -10.00, ImageURL: "", SourceURL: ""},
	}

	valid, rejected := crawler.ValidateProducts(raw)
	assert.Empty(t, valid)
	require.Len(t, rejected, 1)
	assert.Contains(t, rejected[0].Reason, "price")
}

func TestValidateProducts_InvalidImageURL(t *testing.T) {
	raw := []crawler.RawProduct{
		{Name: "No Image", Price: 100.00, ImageURL: "not-a-url", SourceURL: "https://shop.com/item"},
	}

	// Invalid image URL gets replaced with empty string (placeholder fallback),
	// product is still valid.
	valid, rejected := crawler.ValidateProducts(raw)
	require.Len(t, valid, 1)
	assert.Empty(t, rejected)
	assert.Equal(t, "", valid[0].ImageURL)
}

func TestValidateProducts_Deduplication(t *testing.T) {
	raw := []crawler.RawProduct{
		{Name: "iPhone 15 Pro", Price: 5499.00, ImageURL: "", SourceURL: "https://shop.com/a"},
		{Name: "iphone 15 pro", Price: 5599.00, ImageURL: "", SourceURL: "https://shop.com/b"},
		{Name: "  iPhone  15  Pro  ", Price: 5299.00, ImageURL: "", SourceURL: "https://shop.com/c"},
	}

	valid, rejected := crawler.ValidateProducts(raw)
	assert.Len(t, valid, 1)
	// First occurrence is kept
	assert.Equal(t, "iPhone 15 Pro", valid[0].Name)
	assert.Equal(t, 5499.00, valid[0].Price)
	require.Len(t, rejected, 2)
	assert.Contains(t, rejected[0].Reason, "duplicate")
}

func TestValidateProducts_MixedValidInvalid(t *testing.T) {
	raw := []crawler.RawProduct{
		{Name: "Valid Product", Price: 100.00, ImageURL: "https://img.com/valid.jpg", SourceURL: ""},
		{Name: "", Price: 50.00, ImageURL: "", SourceURL: ""},
		{Name: "Zero Price", Price: 0, ImageURL: "", SourceURL: ""},
		{Name: "Another Valid", Price: 200.00, ImageURL: "", SourceURL: ""},
	}

	valid, rejected := crawler.ValidateProducts(raw)
	assert.Len(t, valid, 2)
	assert.Len(t, rejected, 2)
}

func TestNormalizeName(t *testing.T) {
	assert.Equal(t, "iphone 15 pro", crawler.NormalizeName("  iPhone  15  Pro  "))
	assert.Equal(t, "laptop dell xps", crawler.NormalizeName("Laptop Dell XPS"))
	assert.Equal(t, "test", crawler.NormalizeName("  TEST  "))
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/crawler/ -v -run "TestValidate|TestNormalize" -count=1`
Expected: FAIL - `crawler.ValidateProducts` undefined.

- [ ] **Step 3: Implement Validator**

Create `backend/internal/crawler/validator.go`:

```go
package crawler

import (
	"fmt"
	"net/url"
	"strings"
)

// RejectedProduct records why a product was rejected during validation.
type RejectedProduct struct {
	Product RawProduct
	Reason  string
}

// ValidateProducts validates and deduplicates a slice of raw products.
// Returns valid products and rejected products with reasons.
func ValidateProducts(raw []RawProduct) (valid []RawProduct, rejected []RejectedProduct) {
	seen := make(map[string]bool)

	for _, p := range raw {
		// Validate name
		if strings.TrimSpace(p.Name) == "" {
			rejected = append(rejected, RejectedProduct{
				Product: p,
				Reason:  "empty name",
			})
			continue
		}

		// Validate price
		if p.Price <= 0 {
			rejected = append(rejected, RejectedProduct{
				Product: p,
				Reason:  fmt.Sprintf("invalid price: %.2f (must be > 0)", p.Price),
			})
			continue
		}

		// Validate and fix image URL
		if p.ImageURL != "" {
			if _, err := url.ParseRequestURI(p.ImageURL); err != nil {
				// Invalid URL -> replace with empty (placeholder fallback)
				p.ImageURL = ""
			} else if !strings.HasPrefix(p.ImageURL, "http://") && !strings.HasPrefix(p.ImageURL, "https://") {
				p.ImageURL = ""
			}
		}

		// Deduplicate by normalized name
		normalized := NormalizeName(p.Name)
		if seen[normalized] {
			rejected = append(rejected, RejectedProduct{
				Product: p,
				Reason:  fmt.Sprintf("duplicate name (normalized: %q)", normalized),
			})
			continue
		}
		seen[normalized] = true

		valid = append(valid, p)
	}

	return valid, rejected
}

// NormalizeName normalizes a product name for deduplication:
// lowercase, collapse whitespace, trim.
func NormalizeName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ToLower(name)

	// Collapse multiple spaces into one
	parts := strings.Fields(name)
	return strings.Join(parts, " ")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/crawler/ -v -run "TestValidate|TestNormalize" -count=1`
Expected: All 8 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat(crawler): add product validator with deduplication"
```

---

### Task 5: CrawlLogger (Per-Crawl File Logging)

**Files:**
- Create: `backend/internal/crawler/logger.go`
- Create: `backend/internal/crawler/logger_test.go`

Structured per-crawl logging with log levels: FETCH, PARSE, AI_REQUEST, AI_RESPONSE, NAVIGATE, PRODUCT_FOUND, VALIDATION, ERROR.

- [ ] **Step 1: Write failing tests for CrawlLogger**

Create `backend/internal/crawler/logger_test.go`:

```go
package crawler_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCrawlLogger_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	crawlID := uuid.New()

	logger, err := crawler.NewCrawlLogger(tmpDir, crawlID)
	require.NoError(t, err)
	defer logger.Close()

	expectedPath := filepath.Join(tmpDir, "crawl_"+crawlID.String()+".log")
	assert.Equal(t, expectedPath, logger.FilePath())

	_, err = os.Stat(expectedPath)
	assert.NoError(t, err, "log file should exist")
}

func TestCrawlLogger_WritesEntries(t *testing.T) {
	tmpDir := t.TempDir()
	crawlID := uuid.New()

	logger, err := crawler.NewCrawlLogger(tmpDir, crawlID)
	require.NoError(t, err)

	logger.Log("FETCH", "Fetching https://example.com, status 200, 15000 bytes")
	logger.Log("PARSE", "Found 2 JSON-LD products")
	logger.Log("PRODUCT_FOUND", "Product: Laptop Dell, Price: 5999.99")
	logger.Log("ERROR", "Failed to parse page: invalid HTML")
	logger.Close()

	content, err := os.ReadFile(logger.FilePath())
	require.NoError(t, err)

	text := string(content)
	assert.Contains(t, text, "[FETCH]")
	assert.Contains(t, text, "https://example.com")
	assert.Contains(t, text, "[PARSE]")
	assert.Contains(t, text, "[PRODUCT_FOUND]")
	assert.Contains(t, text, "[ERROR]")

	// Each line should have a timestamp
	lines := strings.Split(strings.TrimSpace(text), "\n")
	assert.Len(t, lines, 4)
	for _, line := range lines {
		// Timestamp format: 2026-04-09T12:00:00Z
		assert.Regexp(t, `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`, line)
	}
}

func TestCrawlLogger_CreatesDirectoryIfNeeded(t *testing.T) {
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "logs", "crawls")
	crawlID := uuid.New()

	logger, err := crawler.NewCrawlLogger(nestedDir, crawlID)
	require.NoError(t, err)
	defer logger.Close()

	logger.Log("FETCH", "test entry")
	logger.Close()

	content, err := os.ReadFile(logger.FilePath())
	require.NoError(t, err)
	assert.Contains(t, string(content), "[FETCH]")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/crawler/ -v -run TestCrawlLogger -count=1`
Expected: FAIL - `crawler.NewCrawlLogger` undefined.

- [ ] **Step 3: Implement CrawlLogger**

Create `backend/internal/crawler/logger.go`:

```go
package crawler

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// CrawlLogger writes structured log entries to a per-crawl log file.
type CrawlLogger struct {
	file     *os.File
	filePath string
	mu       sync.Mutex
}

// NewCrawlLogger creates a new logger that writes to logs/crawl_<crawlID>.log.
// Creates the directory if it doesn't exist.
func NewCrawlLogger(logDir string, crawlID uuid.UUID) (*CrawlLogger, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("creating log directory %s: %w", logDir, err)
	}

	filename := fmt.Sprintf("crawl_%s.log", crawlID.String())
	filePath := filepath.Join(logDir, filename)

	file, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("creating log file %s: %w", filePath, err)
	}

	return &CrawlLogger{
		file:     file,
		filePath: filePath,
	}, nil
}

// Log writes a structured log entry with timestamp and level.
// Level should be one of: FETCH, PARSE, AI_REQUEST, AI_RESPONSE, NAVIGATE,
// PRODUCT_FOUND, VALIDATION, ERROR.
func (l *CrawlLogger) Log(level string, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().UTC().Format(time.RFC3339)
	entry := fmt.Sprintf("%s [%s] %s\n", timestamp, level, message)

	l.file.WriteString(entry)
}

// FilePath returns the path to the log file.
func (l *CrawlLogger) FilePath() string {
	return l.filePath
}

// Close closes the log file.
func (l *CrawlLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/crawler/ -v -run TestCrawlLogger -count=1`
Expected: All 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat(crawler): add per-crawl file logger with structured log levels"
```

---

### Task 6: AI Client (Interface + OpenAI SDK Implementation)

**Files:**
- Create: `backend/internal/crawler/ai_client.go`
- Create: `backend/internal/crawler/ai_client_test.go`

The AI client sends HTML to GPT-5 mini and receives structured product data. We define an interface so the crawler can be tested with a mock. The OpenAI implementation uses the **official `openai-go` SDK** (`github.com/openai/openai-go/v3`) which provides type safety, automatic retries, and structured outputs. Tests use `httptest` servers with `option.WithBaseURL` to mock the API.

- [ ] **Step 1: Install OpenAI Go SDK**

```bash
cd backend && go get github.com/openai/openai-go/v3
```

- [ ] **Step 2: Write failing tests for AIClient**

Create `backend/internal/crawler/ai_client_test.go`:

```go
package crawler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIClient_ExtractProducts(t *testing.T) {
	// Mock OpenAI API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request structure
		assert.Equal(t, "POST", r.Method)
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer ")

		// Return a mock response with extracted products
		resp := map[string]interface{}{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"model":   "gpt-5-mini",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": `[{"name":"Laptop Dell XPS 15","price":5999.99,"image_url":"https://img.com/dell.jpg"},{"name":"iPhone 15 Pro","price":5499.00,"image_url":"https://img.com/iphone.jpg"}]`,
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     100,
				"completion_tokens": 400,
				"total_tokens":      500,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := crawler.NewOpenAIClient("test-api-key", "gpt-5-mini", server.URL)
	ctx := context.Background()

	products, tokensUsed, err := client.ExtractProducts(ctx, "<html><body>Product page HTML</body></html>", "https://shop.com/products")
	require.NoError(t, err)
	assert.Len(t, products, 2)
	assert.Equal(t, "Laptop Dell XPS 15", products[0].Name)
	assert.Equal(t, 5999.99, products[0].Price)
	assert.Equal(t, "iPhone 15 Pro", products[1].Name)
	assert.Equal(t, 500, tokensUsed)
}

func TestOpenAIClient_ExtractLinks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"model":   "gpt-5-mini",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": `["/products/laptops","/products/phones","/category/electronics"]`,
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     50,
				"completion_tokens": 150,
				"total_tokens":      200,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := crawler.NewOpenAIClient("test-api-key", "gpt-5-mini", server.URL)
	ctx := context.Background()

	links, tokensUsed, err := client.ExtractLinks(ctx, "<html><body>Category page</body></html>", "https://shop.com")
	require.NoError(t, err)
	assert.Len(t, links, 3)
	assert.Equal(t, "/products/laptops", links[0])
	assert.Equal(t, 200, tokensUsed)
}

func TestOpenAIClient_HandlesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit exceeded","type":"rate_limit_error"}}`))
	}))
	defer server.Close()

	client := crawler.NewOpenAIClient("test-api-key", "gpt-5-mini", server.URL)
	ctx := context.Background()

	_, _, err := client.ExtractProducts(ctx, "<html></html>", "https://shop.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "429")
}

func TestOpenAIClient_HandlesInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"model":   "gpt-5-mini",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "This is not valid JSON at all",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     50,
				"completion_tokens": 50,
				"total_tokens":      100,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := crawler.NewOpenAIClient("test-api-key", "gpt-5-mini", server.URL)
	ctx := context.Background()

	products, _, err := client.ExtractProducts(ctx, "<html></html>", "https://shop.com")
	require.NoError(t, err)
	// Invalid JSON from AI -> empty result, not an error
	assert.Empty(t, products)
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd backend && go test ./internal/crawler/ -v -run TestOpenAIClient -count=1`
Expected: FAIL - `crawler.NewOpenAIClient` undefined.

- [ ] **Step 4: Implement AIClient using official openai-go SDK**

Create `backend/internal/crawler/ai_client.go`:

```go
package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// AIClient is the interface for AI-powered product extraction.
type AIClient interface {
	// ExtractProducts sends HTML to AI and gets structured product data back.
	// Returns products, tokens used, and error.
	ExtractProducts(ctx context.Context, html string, pageURL string) ([]RawProduct, int, error)
	// ExtractLinks sends HTML to AI and gets promising product/category links.
	// Returns link paths, tokens used, and error.
	ExtractLinks(ctx context.Context, html string, baseURL string) ([]string, int, error)
}

// OpenAIClient implements AIClient using the official OpenAI Go SDK.
type OpenAIClient struct {
	client *openai.Client
	model  string
}

// NewOpenAIClient creates an OpenAI client using the official SDK.
// model is the model name (e.g. "gpt-5-mini").
// baseURL is optional - pass "" for production, or a test server URL for testing.
func NewOpenAIClient(apiKey string, model string, baseURL string) *OpenAIClient {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	return &OpenAIClient{
		client: openai.NewClient(opts...),
		model:  model,
	}
}

func (c *OpenAIClient) ExtractProducts(ctx context.Context, html string, pageURL string) ([]RawProduct, int, error) {
	truncatedHTML := truncateHTML(html, 100000)

	prompt := fmt.Sprintf(`You are a product data extractor. Analyze the following HTML from %s and extract product information.

Return a JSON array of products. Each product should have:
- "name": product name (string)
- "price": product price as a number (float, in the page's currency)
- "image_url": URL of the product image (string, empty if not found)

Return ONLY the JSON array, no other text. If no products are found, return an empty array [].

HTML:
%s`, pageURL, truncatedHTML)

	completion, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
		Model:       c.model,
		Temperature: openai.Float(0.1),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("OpenAI API error: %w", err)
	}

	tokensUsed := int(completion.Usage.TotalTokens)

	if len(completion.Choices) == 0 {
		return nil, tokensUsed, fmt.Errorf("no choices in API response")
	}

	content := completion.Choices[0].Message.Content

	var products []RawProduct
	cleaned := cleanJSONResponse(content)
	if err := json.Unmarshal([]byte(cleaned), &products); err != nil {
		// AI returned invalid JSON - not an error, just no products
		return nil, tokensUsed, nil
	}

	// Set source URL for all products
	for i := range products {
		products[i].SourceURL = pageURL
	}

	return products, tokensUsed, nil
}

func (c *OpenAIClient) ExtractLinks(ctx context.Context, html string, baseURL string) ([]string, int, error) {
	truncatedHTML := truncateHTML(html, 100000)

	prompt := fmt.Sprintf(`You are a web crawler assistant. Analyze the following HTML from %s and identify links that likely lead to:
1. Product pages (individual products with prices)
2. Product category/listing pages (containing multiple products)

Return a JSON array of URL paths (relative or absolute). Focus on links that are most likely to contain product data.
Return ONLY the JSON array, no other text. Return at most 20 links. If no relevant links found, return [].

HTML:
%s`, baseURL, truncatedHTML)

	completion, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
		Model:       c.model,
		Temperature: openai.Float(0.1),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("OpenAI API error: %w", err)
	}

	tokensUsed := int(completion.Usage.TotalTokens)

	if len(completion.Choices) == 0 {
		return nil, tokensUsed, fmt.Errorf("no choices in API response")
	}

	content := completion.Choices[0].Message.Content

	var links []string
	cleaned := cleanJSONResponse(content)
	if err := json.Unmarshal([]byte(cleaned), &links); err != nil {
		return nil, tokensUsed, nil
	}

	return links, tokensUsed, nil
}

// truncateHTML truncates HTML content to maxChars characters.
func truncateHTML(html string, maxChars int) string {
	if len(html) <= maxChars {
		return html
	}
	return html[:maxChars]
}

// cleanJSONResponse strips markdown code fences from AI response if present.
func cleanJSONResponse(content string) string {
	content = strings.TrimSpace(content)

	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	}

	return content
}
```

Note: The SDK provides `option.WithBaseURL` for testing, so tests use the same mock `httptest` server approach but through the SDK's built-in mechanism. The SDK also supports Structured Outputs via JSON schema, which could be used in the future for even more reliable product extraction - for now we keep the JSON-in-content approach for simplicity.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd backend && go test ./internal/crawler/ -v -run TestOpenAIClient -count=1`
Expected: All 4 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(crawler): add AI client interface and OpenAI SDK implementation"
```

---

### Task 7: Crawler Orchestrator

**Files:**
- Modify: `backend/internal/crawler/crawler.go` (add Crawler struct and Run method)
- Modify: `backend/internal/store/crawl_store.go` (add UpdateLogPath method)
- Create: `backend/internal/crawler/crawler_test.go`

The orchestrator ties all components together: fetch pages, try structured extraction, fall back to AI, validate, persist to DB. This is the core of Plan 2.

We also need to extend `CrawlStore` with an `UpdateLogPath` method: the crawl record is created first (to get the DB-generated UUID), then the logger is created using that UUID, and finally the log path is saved back.

- [ ] **Step 1: Add UpdateLogPath to CrawlStore**

Add to `backend/internal/store/crawl_store.go` (after the existing `Finish` method):

```go
func (s *CrawlStore) UpdateLogPath(ctx context.Context, id uuid.UUID, logFilePath string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE crawls SET log_file_path = $1 WHERE id = $2`,
		logFilePath, id,
	)
	return err
}
```

- [ ] **Step 2: Write failing tests for Crawler**

Create `backend/internal/crawler/crawler_test.go`:

```go
package crawler_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/jzy/howmuchyousay/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock AI Client ---

type mockAIClient struct {
	extractProductsFn func(ctx context.Context, html string, pageURL string) ([]crawler.RawProduct, int, error)
	extractLinksFn    func(ctx context.Context, html string, baseURL string) ([]string, int, error)
}

func (m *mockAIClient) ExtractProducts(ctx context.Context, html string, pageURL string) ([]crawler.RawProduct, int, error) {
	if m.extractProductsFn != nil {
		return m.extractProductsFn(ctx, html, pageURL)
	}
	return nil, 0, nil
}

func (m *mockAIClient) ExtractLinks(ctx context.Context, html string, baseURL string) ([]string, int, error) {
	if m.extractLinksFn != nil {
		return m.extractLinksFn(ctx, html, baseURL)
	}
	return nil, 0, nil
}

// --- Test DB setup (same as store tests - consider extracting to internal/testutil/) ---

func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable"
	}

	if err := store.RunMigrations(dbURL, "../../migrations"); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	pool, err := store.ConnectDB(dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	t.Cleanup(func() {
		tables := []string{"answers", "rounds", "players", "products", "crawls", "game_sessions", "shops"}
		for _, table := range tables {
			pool.Exec(context.Background(), "DELETE FROM "+table)
		}
		pool.Close()
	})

	return pool
}

func TestCrawler_RunWithStructuredData(t *testing.T) {
	// HTTP server that serves pages with JSON-LD product data
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			w.Write([]byte("User-agent: *\nAllow: /\n"))
		case "/":
			w.Write([]byte(`<html><head>
				<script type="application/ld+json">
				[
					{"@type":"Product","name":"Product 1","offers":{"price":"100.00"},"image":"https://img.com/1.jpg"},
					{"@type":"Product","name":"Product 2","offers":{"price":"200.00"},"image":"https://img.com/2.jpg"},
					{"@type":"Product","name":"Product 3","offers":{"price":"300.00"},"image":"https://img.com/3.jpg"},
					{"@type":"Product","name":"Product 4","offers":{"price":"400.00"},"image":"https://img.com/4.jpg"},
					{"@type":"Product","name":"Product 5","offers":{"price":"500.00"},"image":"https://img.com/5.jpg"}
				]
				</script>
				</head><body>
				<a href="/page2">Page 2</a>
				</body></html>`))
		case "/page2":
			w.Write([]byte(`<html><head>
				<script type="application/ld+json">
				[
					{"@type":"Product","name":"Product 6","offers":{"price":"600.00"},"image":"https://img.com/6.jpg"},
					{"@type":"Product","name":"Product 7","offers":{"price":"700.00"},"image":"https://img.com/7.jpg"},
					{"@type":"Product","name":"Product 8","offers":{"price":"800.00"},"image":"https://img.com/8.jpg"},
					{"@type":"Product","name":"Product 9","offers":{"price":"900.00"},"image":"https://img.com/9.jpg"},
					{"@type":"Product","name":"Product 10","offers":{"price":"1000.00"},"image":"https://img.com/10.jpg"}
				]
				</script>
				</head><body>
				<a href="/page3">Page 3</a>
				</body></html>`))
		case "/page3":
			// Generate enough products to hit minProducts=20
			products := ""
			for i := 11; i <= 25; i++ {
				products += fmt.Sprintf(
					`{"@type":"Product","name":"Product %d","offers":{"price":"%d.00"},"image":"https://img.com/%d.jpg"},`,
					i, i*100, i)
			}
			// Remove trailing comma
			products = products[:len(products)-1]
			w.Write([]byte(fmt.Sprintf(`<html><head>
				<script type="application/ld+json">[%s]</script>
				</head><body></body></html>`, products)))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	pool := setupTestDB(t)
	logDir := t.TempDir()

	mockAI := &mockAIClient{
		extractLinksFn: func(ctx context.Context, html string, baseURL string) ([]string, int, error) {
			// AI suggests links if structured data didn't yield enough
			return []string{"/page2", "/page3"}, 100, nil
		},
	}

	c := crawler.New(
		crawler.NewHTTPFetcher(5*time.Second),
		mockAI,
		store.NewShopStore(pool),
		store.NewCrawlStore(pool),
		store.NewProductStore(pool),
		logDir,
	)

	cfg := crawler.CrawlConfig{
		URL:         server.URL,
		Timeout:     30 * time.Second,
		MinProducts: 20,
		Verbose:     false,
	}

	result, err := c.Run(context.Background(), cfg, nil)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, result.ProductsFound, 20)
	assert.Greater(t, result.PagesVisited, 0)
	assert.NotEmpty(t, result.CrawlID)
	assert.NotEmpty(t, result.ShopID)
}

func TestCrawler_RunWithAIFallback(t *testing.T) {
	// HTTP server with NO structured data - requires AI extraction
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			w.WriteHeader(http.StatusNotFound)
		case "/":
			w.Write([]byte(`<html><body>
				<h1>Shop Products</h1>
				<div class="product"><span>Laptop Dell</span><span>5999 PLN</span></div>
				<div class="product"><span>iPhone 15</span><span>4999 PLN</span></div>
				<a href="/page2">More products</a>
				</body></html>`))
		case "/page2":
			w.Write([]byte(`<html><body>
				<div class="product"><span>Samsung Galaxy</span><span>3999 PLN</span></div>
				</body></html>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	pool := setupTestDB(t)
	logDir := t.TempDir()

	callCount := 0
	mockAI := &mockAIClient{
		extractProductsFn: func(ctx context.Context, html string, pageURL string) ([]crawler.RawProduct, int, error) {
			callCount++
			// Generate 12 unique products per page to exceed minProducts
			var products []crawler.RawProduct
			base := (callCount - 1) * 12
			for i := 0; i < 12; i++ {
				products = append(products, crawler.RawProduct{
					Name:     fmt.Sprintf("AI Product %d", base+i+1),
					Price:    float64((base+i+1)*100) + 0.99,
					ImageURL: fmt.Sprintf("https://img.com/ai-%d.jpg", base+i+1),
				})
			}
			return products, 300, nil
		},
		extractLinksFn: func(ctx context.Context, html string, baseURL string) ([]string, int, error) {
			return []string{"/page2"}, 100, nil
		},
	}

	c := crawler.New(
		crawler.NewHTTPFetcher(5*time.Second),
		mockAI,
		store.NewShopStore(pool),
		store.NewCrawlStore(pool),
		store.NewProductStore(pool),
		logDir,
	)

	cfg := crawler.CrawlConfig{
		URL:         server.URL,
		Timeout:     30 * time.Second,
		MinProducts: 20,
		Verbose:     false,
	}

	result, err := c.Run(context.Background(), cfg, nil)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, result.ProductsFound, 20)
	assert.Greater(t, result.AIRequestsCount, 0)
}

func TestCrawler_RunRespectsTimeout(t *testing.T) {
	// HTTP server that responds slowly
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		time.Sleep(100 * time.Millisecond)
		w.Write([]byte(`<html><body>
			<a href="/page2">next</a>
			</body></html>`))
	}))
	defer server.Close()

	pool := setupTestDB(t)
	logDir := t.TempDir()

	mockAI := &mockAIClient{
		extractLinksFn: func(ctx context.Context, html string, baseURL string) ([]string, int, error) {
			return []string{"/page2"}, 100, nil
		},
	}

	c := crawler.New(
		crawler.NewHTTPFetcher(5*time.Second),
		mockAI,
		store.NewShopStore(pool),
		store.NewCrawlStore(pool),
		store.NewProductStore(pool),
		logDir,
	)

	cfg := crawler.CrawlConfig{
		URL:         server.URL,
		Timeout:     500 * time.Millisecond,
		MinProducts: 100, // Impossible to reach
		Verbose:     false,
	}

	result, err := c.Run(context.Background(), cfg, nil)
	// Timeout is not an error - we just return what we have
	require.NoError(t, err)
	// Result should exist even with few/no products (crawl completed with timeout)
	assert.NotEmpty(t, result.CrawlID)
}

func TestCrawler_RunSavesToDatabase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			w.WriteHeader(http.StatusNotFound)
		case "/":
			w.Write([]byte(`<html><head>
				<script type="application/ld+json">
				{"@type":"Product","name":"DB Test Product","offers":{"price":"99.99"},"image":"https://img.com/db.jpg"}
				</script>
				</head><body></body></html>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	pool := setupTestDB(t)
	logDir := t.TempDir()

	mockAI := &mockAIClient{}

	shopStore := store.NewShopStore(pool)
	crawlStore := store.NewCrawlStore(pool)
	productStore := store.NewProductStore(pool)

	c := crawler.New(
		crawler.NewHTTPFetcher(5*time.Second),
		mockAI,
		shopStore,
		crawlStore,
		productStore,
		logDir,
	)

	cfg := crawler.CrawlConfig{
		URL:         server.URL,
		Timeout:     10 * time.Second,
		MinProducts: 1,
		Verbose:     false,
	}

	result, err := c.Run(context.Background(), cfg, nil)
	require.NoError(t, err)

	// Verify shop was created
	ctx := context.Background()
	shop, err := shopStore.GetByID(ctx, result.ShopID)
	require.NoError(t, err)
	require.NotNil(t, shop)

	// Verify crawl was created and finished
	crawl, err := crawlStore.GetByID(ctx, result.CrawlID)
	require.NoError(t, err)
	require.NotNil(t, crawl)
	assert.Equal(t, "completed", string(crawl.Status))
	assert.Equal(t, result.ProductsFound, crawl.ProductsFound)

	// Verify products were saved
	products, err := productStore.GetByShopID(ctx, result.ShopID)
	require.NoError(t, err)
	assert.Len(t, products, result.ProductsFound)
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/crawler/ -v -run "TestCrawler_" -count=1`
Expected: FAIL - `crawler.New` undefined.

- [ ] **Step 4: Implement Crawler orchestrator**

Modify `backend/internal/crawler/crawler.go` - add the Crawler struct and Run method **after** the existing types:

```go
package crawler

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jzy/howmuchyousay/internal/models"
	"github.com/jzy/howmuchyousay/internal/store"
)

// RawProduct represents a product extracted from a web page before validation.
type RawProduct struct {
	Name      string  `json:"name"`
	Price     float64 `json:"price"`
	ImageURL  string  `json:"image_url"`
	SourceURL string  `json:"source_url"`
}

// CrawlConfig holds settings for a single crawl run.
type CrawlConfig struct {
	URL         string
	Timeout     time.Duration
	MinProducts int
	Verbose     bool
}

// DefaultCrawlConfig returns config with sensible defaults.
func DefaultCrawlConfig(url string) CrawlConfig {
	return CrawlConfig{
		URL:         url,
		Timeout:     90 * time.Second,
		MinProducts: 20,
		Verbose:     false,
	}
}

// CrawlResult contains the outcome of a crawl run.
type CrawlResult struct {
	CrawlID         uuid.UUID
	ShopID          uuid.UUID
	ProductsFound   int
	PagesVisited    int
	AIRequestsCount int
	Duration        time.Duration
	LogFilePath     string
}

// Crawler orchestrates the product crawling pipeline.
type Crawler struct {
	fetcher      Fetcher
	ai           AIClient
	shopStore    *store.ShopStore
	crawlStore   *store.CrawlStore
	productStore *store.ProductStore
	logDir       string
}

// New creates a new Crawler with all dependencies.
func New(
	fetcher Fetcher,
	ai AIClient,
	shopStore *store.ShopStore,
	crawlStore *store.CrawlStore,
	productStore *store.ProductStore,
	logDir string,
) *Crawler {
	return &Crawler{
		fetcher:      fetcher,
		ai:           ai,
		shopStore:    shopStore,
		crawlStore:   crawlStore,
		productStore: productStore,
		logDir:       logDir,
	}
}

// Run executes the full crawl pipeline for the given config.
// sessionID is optional (nil for CLI crawls, non-nil for game-triggered crawls).
func (c *Crawler) Run(ctx context.Context, cfg CrawlConfig, sessionID *uuid.UUID) (*CrawlResult, error) {
	return c.RunWithProgress(ctx, cfg, sessionID, nil)
}

// RunWithProgress executes the full crawl pipeline with optional progress reporting.
func (c *Crawler) RunWithProgress(ctx context.Context, cfg CrawlConfig, sessionID *uuid.UUID, progress ProgressFunc) (*CrawlResult, error) {
	start := time.Now()

	report := func(event, msg string) {
		if progress != nil {
			progress(event, msg)
		}
	}

	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	// 1. Find or create shop
	shop, err := c.findOrCreateShop(ctx, cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("finding/creating shop: %w", err)
	}

	// 2. Create crawl record first to get the DB-generated ID,
	// then create the logger with the actual crawl ID.
	crawl, err := c.crawlStore.Create(ctx, shop.ID, sessionID, "")
	if err != nil {
		return nil, fmt.Errorf("creating crawl record: %w", err)
	}

	logger, err := NewCrawlLogger(c.logDir, crawl.ID)
	if err != nil {
		return nil, fmt.Errorf("creating crawl logger: %w", err)
	}
	defer logger.Close()

	// Save log file path back to crawl record
	if err := c.crawlStore.UpdateLogPath(ctx, crawl.ID, logger.FilePath()); err != nil {
		return nil, fmt.Errorf("updating crawl log path: %w", err)
	}

	// Update crawl status to in_progress
	if err := c.crawlStore.UpdateStatus(ctx, crawl.ID, models.CrawlStatusInProgress); err != nil {
		return nil, fmt.Errorf("updating crawl status: %w", err)
	}

	logger.Log("FETCH", fmt.Sprintf("Starting crawl for %s", cfg.URL))
	report("start", fmt.Sprintf("Starting crawl for %s", cfg.URL))

	// 3. Check robots.txt
	parsedURL, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parsing URL: %w", err)
	}

	allowed, err := c.fetcher.CheckRobotsTxt(ctx, cfg.URL, parsedURL.Path)
	if err != nil {
		logger.Log("ERROR", fmt.Sprintf("Failed to check robots.txt: %v", err))
	}
	if !allowed {
		errMsg := "crawling disallowed by robots.txt"
		logger.Log("ERROR", errMsg)
		c.crawlStore.Finish(ctx, crawl.ID, models.CrawlStatusFailed, 0, 0, 0, &errMsg)
		return &CrawlResult{
			CrawlID:     crawl.ID,
			ShopID:      shop.ID,
			Duration:    time.Since(start),
			LogFilePath: logger.FilePath(),
		}, nil
	}

	// 4. Crawl pages and collect products
	var allProducts []RawProduct
	pagesVisited := 0
	aiRequestsCount := 0
	visited := make(map[string]bool)
	toVisit := []string{cfg.URL}

	for len(toVisit) > 0 && len(allProducts) < cfg.MinProducts*2 {
		// Check context (timeout)
		if ctx.Err() != nil {
			logger.Log("FETCH", "Timeout reached, stopping crawl")
			break
		}

		currentURL := toVisit[0]
		toVisit = toVisit[1:]

		if visited[currentURL] {
			continue
		}
		visited[currentURL] = true

		// Fetch page
		html, err := c.fetcher.Fetch(ctx, currentURL)
		if err != nil {
			logger.Log("ERROR", fmt.Sprintf("Failed to fetch %s: %v", currentURL, err))
			continue
		}
		pagesVisited++
		logger.Log("FETCH", fmt.Sprintf("Fetched %s, %d bytes", currentURL, len(html)))
		report("fetch", fmt.Sprintf("Page %d: %s (%d bytes)", pagesVisited, currentURL, len(html)))

		// Try structured data extraction first
		products := ExtractProducts(html, currentURL)
		if len(products) > 0 {
			logger.Log("PARSE", fmt.Sprintf("Found %d products via structured data on %s", len(products), currentURL))
		}

		// If no structured data, try AI extraction
		if len(products) == 0 && c.ai != nil {
			logger.Log("AI_REQUEST", fmt.Sprintf("No structured data on %s, sending to AI", currentURL))
			aiProducts, tokensUsed, err := c.ai.ExtractProducts(ctx, html, currentURL)
			if err != nil {
				logger.Log("ERROR", fmt.Sprintf("AI extraction failed for %s: %v", currentURL, err))
			} else {
				aiRequestsCount++
				logger.Log("AI_RESPONSE", fmt.Sprintf("AI found %d products, used %d tokens", len(aiProducts), tokensUsed))
				products = aiProducts
			}
		}

		// Log found products
		for _, p := range products {
			logger.Log("PRODUCT_FOUND", fmt.Sprintf("Product: %s, Price: %.2f", p.Name, p.Price))
		}
		allProducts = append(allProducts, products...)
		report("products", fmt.Sprintf("Total products so far: %d", len(allProducts)))

		// If we have enough products, stop
		if len(allProducts) >= cfg.MinProducts*2 {
			break
		}

		// Discover more links
		pageLinks := ExtractLinks(html, currentURL)

		// Filter to same-host links only
		var sameHostLinks []string
		for _, link := range pageLinks {
			linkParsed, err := url.Parse(link)
			if err != nil {
				continue
			}
			if linkParsed.Host == parsedURL.Host && !visited[link] {
				sameHostLinks = append(sameHostLinks, link)
			}
		}

		// If few same-host links found, ask AI for guidance
		if len(sameHostLinks) < 3 && c.ai != nil && len(allProducts) < cfg.MinProducts {
			aiLinks, tokensUsed, err := c.ai.ExtractLinks(ctx, html, cfg.URL)
			if err != nil {
				logger.Log("ERROR", fmt.Sprintf("AI link extraction failed: %v", err))
			} else {
				aiRequestsCount++
				logger.Log("NAVIGATE", fmt.Sprintf("AI suggested %d links, used %d tokens", len(aiLinks), tokensUsed))
				for _, link := range aiLinks {
					// Resolve relative links
					resolved, err := parsedURL.Parse(link)
					if err != nil {
						continue
					}
					fullURL := resolved.String()
					if !visited[fullURL] {
						sameHostLinks = append(sameHostLinks, fullURL)
					}
				}
			}
		}

		toVisit = append(toVisit, sameHostLinks...)
	}

	// 5. Validate and deduplicate
	validProducts, rejectedProducts := ValidateProducts(allProducts)
	for _, r := range rejectedProducts {
		logger.Log("VALIDATION", fmt.Sprintf("Rejected: %s - %s", r.Product.Name, r.Reason))
	}
	logger.Log("VALIDATION", fmt.Sprintf("Valid: %d, Rejected: %d", len(validProducts), len(rejectedProducts)))

	// 6. Save products to DB
	savedCount := 0
	for _, p := range validProducts {
		_, err := c.productStore.Create(ctx, shop.ID, crawl.ID, p.Name, p.Price, p.ImageURL, p.SourceURL)
		if err != nil {
			logger.Log("ERROR", fmt.Sprintf("Failed to save product %s: %v", p.Name, err))
			continue
		}
		savedCount++
	}

	// 7. Update shop last_crawled_at
	if err := c.shopStore.UpdateLastCrawled(ctx, shop.ID); err != nil {
		logger.Log("ERROR", fmt.Sprintf("Failed to update shop last_crawled_at: %v", err))
	}

	// 8. Finish crawl record
	duration := time.Since(start)
	status := models.CrawlStatusCompleted
	var errMsg *string
	if savedCount < cfg.MinProducts {
		status = models.CrawlStatusCompleted // Still completed, just with fewer products
		msg := fmt.Sprintf("found only %d products (minimum: %d)", savedCount, cfg.MinProducts)
		errMsg = &msg
		logger.Log("ERROR", msg)
	}

	if err := c.crawlStore.Finish(ctx, crawl.ID, status, savedCount, pagesVisited, aiRequestsCount, errMsg); err != nil {
		logger.Log("ERROR", fmt.Sprintf("Failed to finish crawl record: %v", err))
	}

	logger.Log("FETCH", fmt.Sprintf("Crawl complete: %d products, %d pages, %d AI requests, %v", savedCount, pagesVisited, aiRequestsCount, duration))
	report("done", fmt.Sprintf("Crawl complete: %d products, %d pages", savedCount, pagesVisited))

	return &CrawlResult{
		CrawlID:         crawl.ID,
		ShopID:          shop.ID,
		ProductsFound:   savedCount,
		PagesVisited:    pagesVisited,
		AIRequestsCount: aiRequestsCount,
		Duration:        duration,
		LogFilePath:     logger.FilePath(),
	}, nil
}

// findOrCreateShop looks up or creates a shop by URL.
func (c *Crawler) findOrCreateShop(ctx context.Context, rawURL string) (*models.Shop, error) {
	normalized := normalizeShopURL(rawURL)

	shop, err := c.shopStore.GetByURL(ctx, normalized)
	if err != nil {
		return nil, err
	}
	if shop != nil {
		return shop, nil
	}

	return c.shopStore.Create(ctx, normalized)
}

// normalizeShopURL normalizes a shop URL to a canonical form.
// Strips trailing slashes, lowercases the host, removes query/fragment.
func normalizeShopURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	// Keep scheme and host only for the base URL
	normalized := fmt.Sprintf("%s://%s", parsed.Scheme, strings.ToLower(parsed.Host))
	return normalized
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/crawler/ -v -run "TestCrawler_" -count=1 -timeout 60s`
Expected: All 4 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(crawler): add crawler orchestrator with full crawl pipeline"
```

---

### Task 8: CLI Entry Point

**Files:**
- Create: `backend/cmd/crawler/main.go`

The CLI binary: parses flags, connects to DB, runs crawler, prints results to stdout.

- [ ] **Step 1: Create CLI main.go**

Create `backend/cmd/crawler/main.go`:

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jzy/howmuchyousay/internal/config"
	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/jzy/howmuchyousay/internal/store"
)

func main() {
	urlFlag := flag.String("url", "", "Shop URL to crawl (required)")
	timeoutFlag := flag.Duration("timeout", 90*time.Second, "Maximum crawl duration")
	minProductsFlag := flag.Int("min-products", 20, "Minimum number of products to collect")
	verboseFlag := flag.Bool("verbose", false, "Enable verbose output")

	flag.Parse()

	if *urlFlag == "" {
		fmt.Fprintln(os.Stderr, "Error: --url flag is required")
		fmt.Fprintln(os.Stderr, "Usage: crawler --url <shop_url> [--timeout 90s] [--min-products 20] [--verbose]")
		os.Exit(1)
	}

	cfg := config.Load()

	if cfg.OpenAIAPIKey == "" {
		fmt.Fprintln(os.Stderr, "Warning: OPENAI_API_KEY not set. AI extraction will not be available.")
	}

	// Run migrations
	if err := store.RunMigrations(cfg.DatabaseURL, findMigrationsPath()); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Connect to database
	pool, err := store.ConnectDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	// Create components
	fetcher := crawler.NewHTTPFetcher(*timeoutFlag)

	var aiClient crawler.AIClient
	if cfg.OpenAIAPIKey != "" {
		aiClient = crawler.NewOpenAIClient(cfg.OpenAIAPIKey, cfg.OpenAIModel, "")
	}

	c := crawler.New(
		fetcher,
		aiClient,
		store.NewShopStore(pool),
		store.NewCrawlStore(pool),
		store.NewProductStore(pool),
		cfg.LogDir,
	)

	crawlCfg := crawler.CrawlConfig{
		URL:         *urlFlag,
		Timeout:     *timeoutFlag,
		MinProducts: *minProductsFlag,
		Verbose:     *verboseFlag,
	}

	if *verboseFlag {
		fmt.Printf("Starting crawl of %s\n", *urlFlag)
		fmt.Printf("  Timeout: %s\n", *timeoutFlag)
		fmt.Printf("  Min products: %d\n", *minProductsFlag)
		fmt.Println()
	}

	var progress crawler.ProgressFunc
	if *verboseFlag {
		progress = func(event string, message string) {
			fmt.Printf("[%s] %s\n", event, message)
		}
	}

	result, err := c.RunWithProgress(context.Background(), crawlCfg, nil, progress)
	if err != nil {
		log.Fatalf("Crawl failed: %v", err)
	}

	// Print summary
	fmt.Println("=== Crawl Complete ===")
	fmt.Printf("  Shop ID:      %s\n", result.ShopID)
	fmt.Printf("  Crawl ID:     %s\n", result.CrawlID)
	fmt.Printf("  Products:     %d\n", result.ProductsFound)
	fmt.Printf("  Pages:        %d\n", result.PagesVisited)
	fmt.Printf("  AI requests:  %d\n", result.AIRequestsCount)
	fmt.Printf("  Duration:     %s\n", result.Duration.Round(time.Millisecond))
	fmt.Printf("  Log file:     %s\n", result.LogFilePath)

	if result.ProductsFound < crawlCfg.MinProducts {
		fmt.Printf("\nWarning: Found only %d products (minimum: %d)\n", result.ProductsFound, crawlCfg.MinProducts)
		os.Exit(1)
	}
}

// findMigrationsPath tries common relative paths for the migrations directory.
func findMigrationsPath() string {
	candidates := []string{
		"../migrations",
		"../../migrations",
		"./migrations",
		"backend/migrations",
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
	}
	return "../migrations" // default fallback
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd backend && go build ./cmd/crawler/`
Expected: No errors, binary created.

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "feat(crawler): add CLI entry point with flags and stdout summary"
```

---

### Task 9: Full Test Suite Run & Vet

- [ ] **Step 1: Run all crawler tests**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/crawler/ -v -count=1 -timeout 120s`
Expected: All tests PASS (approximately 24 tests).

- [ ] **Step 2: Run all project tests (store + crawler)**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./... -v -count=1 -timeout 120s`
Expected: All tests PASS.

- [ ] **Step 3: Run go vet**

Run: `cd backend && go vet ./...`
Expected: No issues.

- [ ] **Step 4: Build both binaries**

Run:
```bash
cd backend && go build -o ../bin/server ./cmd/server/ && go build -o ../bin/crawler ./cmd/crawler/
```
Expected: Both binaries build successfully.

- [ ] **Step 5: Final commit if any cleanup was needed**

```bash
git add -A
git commit -m "chore: verify full test suite and both binaries build"
```

(Skip this commit if no changes were needed.)
