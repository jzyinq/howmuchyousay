# Firecrawl Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the custom HTTP/BFS/sitemap crawler with Firecrawl's `/v2/crawl` API while keeping GPT-5 mini product extraction on clean markdown.

**Architecture:** A new `FirecrawlCrawler` interface wraps the `firecrawl-go` SDK. The `Crawler` orchestrator drops its BFS loop and instead calls `FirecrawlCrawler.CrawlSite()` to get all pages at once, then iterates results to extract products via AI. Files `fetcher.go`, `sitemap.go` and their tests are deleted. `ai_client.go` prompts change from HTML to markdown. `extractor.go` is adapted to work with Firecrawl metadata.

**Tech Stack:** Go 1.26.2, `github.com/mendableai/firecrawl-go/v2`, OpenAI Go SDK v3, PostgreSQL, testify

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `backend/internal/crawler/firecrawl_client.go` | Create | `FirecrawlCrawler` interface + `FirecrawlClient` implementation wrapping SDK |
| `backend/internal/crawler/firecrawl_client_test.go` | Create | Unit tests for `FirecrawlClient` with mock HTTP server |
| `backend/internal/crawler/crawler.go` | Rewrite | Drop BFS loop, use `FirecrawlCrawler.CrawlSite()`, iterate `PageResult`s |
| `backend/internal/crawler/crawler_test.go` | Rewrite | Integration tests with mock `FirecrawlCrawler` |
| `backend/internal/crawler/ai_client.go` | Modify | Change prompt from HTML to markdown, remove `ExtractLinks`, remove `truncateHTML` |
| `backend/internal/crawler/ai_client_test.go` | Modify | Update tests for new prompts, remove `ExtractLinks` tests |
| `backend/internal/crawler/extractor.go` | Rewrite | New `ExtractProductsFromPage(PageResult)` using metadata; remove HTML-based functions |
| `backend/internal/crawler/extractor_test.go` | Rewrite | Tests for metadata-based extraction |
| `backend/internal/crawler/logger.go` | Modify | Add `FIRECRAWL_START/PROGRESS/COMPLETE` levels, remove `FETCH` from docstring |
| `backend/internal/crawler/logger_test.go` | Modify | Update for new log levels |
| `backend/internal/crawler/validator.go` | Unchanged | No changes needed |
| `backend/internal/config/config.go` | Modify | Add `FirecrawlAPIKey`, `FirecrawlAPIURL`, `FirecrawlMaxDepth` |
| `backend/cmd/crawler/main.go` | Modify | Create `FirecrawlClient` instead of `HTTPFetcher`, update constructor |
| `backend/.env.example` (at repo root: `.env.example`) | Modify | Add `FIRECRAWL_API_KEY`, `FIRECRAWL_API_URL` |
| `backend/internal/crawler/fetcher.go` | Delete | Replaced by Firecrawl |
| `backend/internal/crawler/fetcher_test.go` | Delete | Tests for deleted code |
| `backend/internal/crawler/sitemap.go` | Delete | Replaced by Firecrawl |
| `backend/internal/crawler/sitemap_test.go` | Delete | Tests for deleted code |

---

### Task 1: Add Firecrawl SDK dependency and update config

**Files:**
- Modify: `backend/go.mod`
- Modify: `backend/internal/config/config.go`
- Modify: `.env.example`

- [ ] **Step 1: Add the Firecrawl Go SDK dependency**

```bash
cd backend && go get github.com/mendableai/firecrawl-go/v2
```

- [ ] **Step 2: Update config.go to add Firecrawl fields**

Replace the full contents of `backend/internal/config/config.go` with:

```go
package config

import (
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL     string
	OpenAIAPIKey    string
	OpenAIModel     string
	LogDir          string
	ServerPort      string
	FirecrawlAPIKey string
	FirecrawlAPIURL string
	FirecrawlMaxDepth int
}

func Load() *Config {
	return &Config{
		DatabaseURL:       getEnv("DATABASE_URL", "postgres://hmys:hmys_dev@localhost:5432/howmuchyousay?sslmode=disable"),
		OpenAIAPIKey:      getEnv("OPENAI_API_KEY", ""),
		OpenAIModel:       getEnv("OPENAI_MODEL", "gpt-5-mini"),
		LogDir:            getEnv("LOG_DIR", "./logs"),
		ServerPort:        getEnv("SERVER_PORT", "8080"),
		FirecrawlAPIKey:   getEnv("FIRECRAWL_API_KEY", ""),
		FirecrawlAPIURL:   getEnv("FIRECRAWL_API_URL", "https://api.firecrawl.dev"),
		FirecrawlMaxDepth: getEnvInt("FIRECRAWL_MAX_DEPTH", 3),
	}
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	val, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return fallback
	}
	return n
}
```

- [ ] **Step 3: Update .env.example**

Add these lines to `.env.example`:

```
FIRECRAWL_API_KEY=fc-your-key-here
FIRECRAWL_API_URL=https://api.firecrawl.dev
FIRECRAWL_MAX_DEPTH=3
```

- [ ] **Step 4: Verify compilation**

```bash
cd backend && go build ./...
```

Expected: Compiles without errors.

- [ ] **Step 5: Commit**

```bash
git add backend/go.mod backend/go.sum backend/internal/config/config.go .env.example
git commit -m "feat: add Firecrawl SDK dependency and config fields"
```

---

### Task 2: Create FirecrawlCrawler interface and client

**Files:**
- Create: `backend/internal/crawler/firecrawl_client.go`
- Create: `backend/internal/crawler/firecrawl_client_test.go`

- [ ] **Step 1: Write the failing test for FirecrawlClient**

Create `backend/internal/crawler/firecrawl_client_test.go`:

```go
package crawler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFirecrawlClient_CrawlSite(t *testing.T) {
	// Mock Firecrawl API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/v2/crawl":
			// Return a crawl job ID
			resp := map[string]interface{}{
				"success": true,
				"id":      "test-job-123",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		case r.Method == "GET" && r.URL.Path == "/v2/crawl/test-job-123":
			// Return completed crawl with page results
			resp := map[string]interface{}{
				"status":    "completed",
				"total":     2,
				"completed": 2,
				"data": []map[string]interface{}{
					{
						"markdown": "# Product Page\n\n## Laptop Dell XPS 15\n\nPrice: 5999.99 PLN\n\n![image](https://img.com/dell.jpg)",
						"metadata": map[string]interface{}{
							"title":       "Laptop Dell XPS 15 - Shop",
							"description": "Buy Laptop Dell XPS 15 for 5999.99 PLN",
							"sourceURL":   "https://shop.com/products/laptop",
							"ogImage":     "https://img.com/dell-og.jpg",
							"statusCode":  200,
						},
					},
					{
						"markdown": "# Another Product\n\n## iPhone 15 Pro\n\nPrice: 5499 PLN",
						"metadata": map[string]interface{}{
							"title":       "iPhone 15 Pro - Shop",
							"description": "Buy iPhone 15 Pro",
							"sourceURL":   "https://shop.com/products/iphone",
							"statusCode":  200,
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	client := crawler.NewFirecrawlClient("test-api-key", server.URL)

	cfg := crawler.CrawlConfig{
		URL:         "https://shop.com",
		Timeout:     30 * time.Second,
		MinProducts: 5,
	}

	pages, err := client.CrawlSite(context.Background(), cfg.URL, cfg)
	require.NoError(t, err)
	require.Len(t, pages, 2)

	assert.Contains(t, pages[0].Markdown, "Laptop Dell XPS 15")
	assert.Equal(t, "https://shop.com/products/laptop", pages[0].URL)
	assert.Equal(t, "Laptop Dell XPS 15 - Shop", pages[0].Metadata["title"])
}

func TestFirecrawlClient_CrawlSite_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"Invalid API key"}`))
	}))
	defer server.Close()

	client := crawler.NewFirecrawlClient("bad-key", server.URL)

	cfg := crawler.CrawlConfig{
		URL:         "https://shop.com",
		Timeout:     10 * time.Second,
		MinProducts: 5,
	}

	_, err := client.CrawlSite(context.Background(), cfg.URL, cfg)
	require.Error(t, err)
}

func TestFirecrawlClient_CrawlSite_EmptyResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/v2/crawl":
			resp := map[string]interface{}{
				"success": true,
				"id":      "test-job-empty",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		case r.Method == "GET" && r.URL.Path == "/v2/crawl/test-job-empty":
			resp := map[string]interface{}{
				"status":    "completed",
				"total":     0,
				"completed": 0,
				"data":      []interface{}{},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	client := crawler.NewFirecrawlClient("test-api-key", server.URL)

	cfg := crawler.CrawlConfig{
		URL:         "https://empty.com",
		Timeout:     10 * time.Second,
		MinProducts: 5,
	}

	pages, err := client.CrawlSite(context.Background(), cfg.URL, cfg)
	require.NoError(t, err)
	assert.Empty(t, pages)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd backend && go test ./internal/crawler/ -run TestFirecrawlClient -v
```

Expected: FAIL — `crawler.NewFirecrawlClient` and `crawler.FirecrawlCrawler` not defined.

- [ ] **Step 3: Implement FirecrawlClient**

Create `backend/internal/crawler/firecrawl_client.go`:

```go
package crawler

import (
	"context"
	"fmt"

	firecrawl "github.com/mendableai/firecrawl-go/v2"
)

// FirecrawlCrawler crawls a website using the Firecrawl API and returns page results.
type FirecrawlCrawler interface {
	// CrawlSite starts a crawl job and blocks until it completes or the context expires.
	// Returns one PageResult per successfully scraped page.
	CrawlSite(ctx context.Context, siteURL string, cfg CrawlConfig) ([]PageResult, error)
}

// PageResult holds the scraped content and metadata for a single page.
type PageResult struct {
	URL      string            // source URL of the page
	Markdown string            // clean markdown content
	Metadata map[string]string // title, description, ogImage, etc.
}

// FirecrawlClient implements FirecrawlCrawler using the official Firecrawl Go SDK.
type FirecrawlClient struct {
	app *firecrawl.FirecrawlApp
}

// NewFirecrawlClient creates a new Firecrawl client.
// apiURL is the Firecrawl API base URL (e.g. "https://api.firecrawl.dev").
func NewFirecrawlClient(apiKey string, apiURL string) *FirecrawlClient {
	app, err := firecrawl.NewFirecrawlApp(apiKey, apiURL)
	if err != nil {
		// The SDK only errors on empty API key, which we validate at startup.
		panic(fmt.Sprintf("firecrawl init: %v", err))
	}
	return &FirecrawlClient{app: app}
}

func (fc *FirecrawlClient) CrawlSite(ctx context.Context, siteURL string, cfg CrawlConfig) ([]PageResult, error) {
	// Build crawl parameters
	maxDepth := 3
	limit := cfg.MinProducts * 3
	if limit < 10 {
		limit = 10
	}

	params := &firecrawl.CrawlParams{
		MaxDepth:     &maxDepth,
		Limit:        &limit,
		ExcludePaths: []string{"blog/*", "about/*", "contact/*", "faq/*", "help/*", "terms/*", "privacy/*", "career*"},
		ScrapeOptions: firecrawl.ScrapeParams{
			Formats: []string{"markdown"},
		},
	}

	// CrawlUrl blocks until the crawl is complete (SDK handles polling).
	crawlResult, err := fc.app.CrawlURL(siteURL, params, nil)
	if err != nil {
		return nil, fmt.Errorf("firecrawl crawl failed: %w", err)
	}

	// Convert SDK results to our PageResult type
	var pages []PageResult
	for _, doc := range crawlResult.Data {
		pr := PageResult{
			Markdown: doc.Markdown,
			Metadata: make(map[string]string),
		}

		// Extract source URL from metadata
		if doc.Metadata != nil {
			if sourceURL, ok := doc.Metadata["sourceURL"].(string); ok {
				pr.URL = sourceURL
			}
			// Copy string metadata fields
			for key, val := range doc.Metadata {
				if s, ok := val.(string); ok {
					pr.Metadata[key] = s
				}
			}
		}

		// Fallback: if no sourceURL in metadata, use the original URL
		if pr.URL == "" {
			pr.URL = siteURL
		}

		// Skip non-200 pages
		if statusStr, ok := pr.Metadata["statusCode"]; ok {
			if statusStr != "200" && statusStr != "" {
				continue
			}
		}

		pages = append(pages, pr)
	}

	return pages, nil
}

// Ensure FirecrawlClient implements FirecrawlCrawler at compile time.
var _ FirecrawlCrawler = (*FirecrawlClient)(nil)
```

Note: The exact SDK types (`CrawlParams`, `ScrapeParams`, `CrawlURL`, response fields like `crawlResult.Data`, `doc.Markdown`, `doc.Metadata`) depend on the actual `firecrawl-go/v2` SDK API. After `go get`, inspect the SDK source (`go doc github.com/mendableai/firecrawl-go/v2`) and adjust field names if needed. The structure above matches the SDK's documented API.

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd backend && go test ./internal/crawler/ -run TestFirecrawlClient -v
```

Expected: All 3 tests PASS. (Note: If the mock server doesn't match the SDK's expected request/response format, you'll need to adjust the mock. The SDK may make requests differently than raw HTTP — check `go doc` output.)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/crawler/firecrawl_client.go backend/internal/crawler/firecrawl_client_test.go
git commit -m "feat: add FirecrawlCrawler interface and client implementation"
```

---

### Task 3: Update AI client for markdown input and remove ExtractLinks

**Files:**
- Modify: `backend/internal/crawler/ai_client.go`
- Modify: `backend/internal/crawler/ai_client_test.go`

- [ ] **Step 1: Update the test for markdown-based extraction**

Replace the full contents of `backend/internal/crawler/ai_client_test.go` with:

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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer ")

		// Verify the request body contains markdown-related prompt text
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)
		messages := reqBody["messages"].([]interface{})
		firstMsg := messages[0].(map[string]interface{})
		content := firstMsg["content"].(string)
		assert.Contains(t, content, "markdown")

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

	markdown := "# Laptop Dell XPS 15\n\nPrice: 5999.99 PLN\n\n![img](https://img.com/dell.jpg)"
	products, tokensUsed, err := client.ExtractProducts(ctx, markdown, "https://shop.com/products")
	require.NoError(t, err)
	assert.Len(t, products, 2)
	assert.Equal(t, "Laptop Dell XPS 15", products[0].Name)
	assert.Equal(t, 5999.99, products[0].Price)
	assert.Equal(t, "https://shop.com/products", products[0].SourceURL)
	assert.Equal(t, 500, tokensUsed)
}

func TestOpenAIClient_HandlesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit exceeded","type":"rate_limit_error"}}`))
	}))
	defer server.Close()

	client := crawler.NewOpenAIClient("test-api-key", "gpt-5-mini", server.URL)
	ctx := context.Background()

	_, _, err := client.ExtractProducts(ctx, "# Some markdown", "https://shop.com")
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

	products, _, err := client.ExtractProducts(ctx, "# Some markdown", "https://shop.com")
	require.NoError(t, err)
	assert.Empty(t, products)
}
```

- [ ] **Step 2: Update ai_client.go — change prompt to markdown, remove ExtractLinks**

Replace the full contents of `backend/internal/crawler/ai_client.go` with:

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
	// ExtractProducts sends page markdown to AI and gets structured product data back.
	// Returns products, tokens used, and error.
	ExtractProducts(ctx context.Context, markdown string, pageURL string) ([]RawProduct, int, error)
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

	client := openai.NewClient(opts...)
	return &OpenAIClient{
		client: &client,
		model:  model,
	}
}

func (c *OpenAIClient) ExtractProducts(ctx context.Context, markdown string, pageURL string) ([]RawProduct, int, error) {
	prompt := fmt.Sprintf(`You are a product data extractor. Analyze the following markdown content from %s and extract product information.

Return a JSON array of products. Each product should have:
- "name": product name (string)
- "price": product price as a number (float, in the page's currency)
- "image_url": URL of the product image (string, empty if not found)

Return ONLY the JSON array, no other text. If no products are found, return an empty array [].

Markdown:
%s`, pageURL, markdown)

	completion, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
		Model: c.model,
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
		return nil, tokensUsed, nil
	}

	// Set source URL for all products
	for i := range products {
		products[i].SourceURL = pageURL
	}

	return products, tokensUsed, nil
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

- [ ] **Step 3: Run tests to verify they pass**

```bash
cd backend && go test ./internal/crawler/ -run TestOpenAIClient -v
```

Expected: All 3 tests PASS.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/crawler/ai_client.go backend/internal/crawler/ai_client_test.go
git commit -m "feat: update AI client prompts for markdown input, remove ExtractLinks"
```

---

### Task 4: Rewrite extractor for Firecrawl PageResult

**Files:**
- Rewrite: `backend/internal/crawler/extractor.go`
- Rewrite: `backend/internal/crawler/extractor_test.go`

- [ ] **Step 1: Write the failing tests for metadata-based extraction**

Replace the full contents of `backend/internal/crawler/extractor_test.go` with:

```go
package crawler_test

import (
	"testing"

	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractProductsFromPage_WithOGProductMetadata(t *testing.T) {
	page := crawler.PageResult{
		URL:      "https://shop.com/product/laptop",
		Markdown: "# Laptop Dell XPS 15\n\nGreat laptop for professionals.\n\nPrice: 5999.99 PLN",
		Metadata: map[string]string{
			"ogTitle": "Laptop Dell XPS 15",
			"ogImage": "https://img.com/dell.jpg",
			"title":   "Laptop Dell XPS 15 - Shop",
		},
	}

	products := crawler.ExtractProductsFromPage(page)
	// OG metadata alone doesn't have price, so no products extracted from metadata alone
	assert.Empty(t, products)
}

func TestExtractProductsFromPage_NoMetadata(t *testing.T) {
	page := crawler.PageResult{
		URL:      "https://shop.com/product/laptop",
		Markdown: "# Laptop Dell XPS 15\n\nPrice: 5999.99 PLN",
		Metadata: map[string]string{},
	}

	products := crawler.ExtractProductsFromPage(page)
	assert.Empty(t, products)
}

func TestExtractProductsFromPage_EmptyMarkdown(t *testing.T) {
	page := crawler.PageResult{
		URL:      "https://shop.com/empty",
		Markdown: "",
		Metadata: map[string]string{
			"title": "Empty Page",
		},
	}

	products := crawler.ExtractProductsFromPage(page)
	assert.Empty(t, products)
}

func TestHasProductSignals_WithProductKeywords(t *testing.T) {
	page := crawler.PageResult{
		URL:      "https://shop.com/product/laptop",
		Markdown: "# Laptop Dell XPS 15\n\nAdd to cart\n\nPrice: 5999.99 PLN",
		Metadata: map[string]string{
			"title": "Laptop Dell XPS 15 - Buy Now",
		},
	}

	assert.True(t, crawler.HasProductSignals(page))
}

func TestHasProductSignals_WithoutProductKeywords(t *testing.T) {
	page := crawler.PageResult{
		URL:      "https://shop.com/about",
		Markdown: "# About Us\n\nWe are a great company founded in 2020.",
		Metadata: map[string]string{
			"title": "About Us - Shop",
		},
	}

	assert.False(t, crawler.HasProductSignals(page))
}

func TestHasProductSignals_WithPricePattern(t *testing.T) {
	page := crawler.PageResult{
		URL:      "https://shop.com/product/phone",
		Markdown: "# Samsung Galaxy S24\n\n4999,99 zł\n\nGreat phone",
		Metadata: map[string]string{},
	}

	assert.True(t, crawler.HasProductSignals(page))
}

func TestPageHasContent(t *testing.T) {
	empty := crawler.PageResult{URL: "https://shop.com", Markdown: ""}
	assert.False(t, crawler.PageHasContent(empty))

	short := crawler.PageResult{URL: "https://shop.com", Markdown: "Hi"}
	assert.False(t, crawler.PageHasContent(short))

	ok := crawler.PageResult{URL: "https://shop.com", Markdown: "# Product\n\nThis is a real product page with enough content to be useful."}
	assert.True(t, crawler.PageHasContent(ok))
}

func TestExtractProductsFromPage_WithOGProduct(t *testing.T) {
	// When og:type is "product" and product:price:amount is in metadata,
	// we can extract a product directly from metadata.
	page := crawler.PageResult{
		URL:      "https://shop.com/product/phone",
		Markdown: "# iPhone 15 Pro\n\nBest phone ever",
		Metadata: map[string]string{
			"ogTitle":              "iPhone 15 Pro",
			"ogImage":             "https://img.com/iphone.jpg",
			"og:type":             "product",
			"product:price:amount": "5499.00",
		},
	}

	products := crawler.ExtractProductsFromPage(page)
	require.Len(t, products, 1)
	assert.Equal(t, "iPhone 15 Pro", products[0].Name)
	assert.Equal(t, 5499.00, products[0].Price)
	assert.Equal(t, "https://img.com/iphone.jpg", products[0].ImageURL)
	assert.Equal(t, "https://shop.com/product/phone", products[0].SourceURL)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd backend && go test ./internal/crawler/ -run "TestExtractProductsFromPage|TestHasProductSignals|TestPageHasContent" -v
```

Expected: FAIL — `ExtractProductsFromPage`, `HasProductSignals`, `PageHasContent` not defined.

- [ ] **Step 3: Implement the new extractor**

Replace the full contents of `backend/internal/crawler/extractor.go` with:

```go
package crawler

import (
	"regexp"
	"strconv"
	"strings"
)

// ExtractProductsFromPage extracts product data from a Firecrawl PageResult.
// Uses OG metadata when available (og:type=product with product:price:amount).
// Returns nil if no structured product data can be extracted from metadata alone.
// AI extraction is handled separately by the crawler orchestrator.
func ExtractProductsFromPage(page PageResult) []RawProduct {
	// Check if metadata contains OG product data
	ogType := page.Metadata["og:type"]
	if strings.EqualFold(ogType, "product") {
		return extractFromOGMetadata(page)
	}

	return nil
}

// extractFromOGMetadata extracts a product from OG metadata fields.
func extractFromOGMetadata(page PageResult) []RawProduct {
	name := page.Metadata["ogTitle"]
	if name == "" {
		name = page.Metadata["title"]
	}
	if name == "" {
		return nil
	}

	priceStr := page.Metadata["product:price:amount"]
	if priceStr == "" {
		return nil
	}

	price := parsePriceValue(priceStr)
	if price <= 0 {
		return nil
	}

	imageURL := page.Metadata["ogImage"]

	return []RawProduct{{
		Name:      name,
		Price:     price,
		ImageURL:  imageURL,
		SourceURL: page.URL,
	}}
}

// HasProductSignals checks if a page likely contains product information
// based on keyword heuristics in the markdown and metadata.
// Used to decide whether to send the page to AI for extraction.
func HasProductSignals(page PageResult) bool {
	text := strings.ToLower(page.Markdown + " " + page.Metadata["title"])

	// Check for e-commerce keywords
	keywords := []string{
		"add to cart", "buy now", "add to basket", "dodaj do koszyka",
		"kup teraz", "cena", "price", "zł", "pln",
	}
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}

	// Check for price patterns (e.g., "5999.99", "4999,99", "$99.99", "99.99 zł")
	pricePattern := regexp.MustCompile(`\d{2,}[.,]\d{2}`)
	if pricePattern.MatchString(page.Markdown) {
		return true
	}

	return false
}

// PageHasContent checks if a page has enough content to be worth processing.
func PageHasContent(page PageResult) bool {
	return len(strings.TrimSpace(page.Markdown)) > 50
}

// parsePriceValue parses a price string into a float64.
func parsePriceValue(v interface{}) float64 {
	switch p := v.(type) {
	case float64:
		return p
	case string:
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd backend && go test ./internal/crawler/ -run "TestExtractProductsFromPage|TestHasProductSignals|TestPageHasContent" -v
```

Expected: All 8 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/crawler/extractor.go backend/internal/crawler/extractor_test.go
git commit -m "feat: rewrite extractor for Firecrawl PageResult metadata"
```

---

### Task 5: Update logger with new log levels

**Files:**
- Modify: `backend/internal/crawler/logger.go`
- Modify: `backend/internal/crawler/logger_test.go`

- [ ] **Step 1: Update the logger test for new log levels**

Replace the full contents of `backend/internal/crawler/logger_test.go` with:

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

	logger.Log("FIRECRAWL_START", "Starting Firecrawl job for https://example.com")
	logger.Log("FIRECRAWL_PROGRESS", "Crawled 5/10 pages")
	logger.Log("FIRECRAWL_COMPLETE", "Firecrawl job finished, 10 pages returned")
	logger.Log("AI_REQUEST", "Sending markdown to AI for extraction")
	logger.Log("PRODUCT_FOUND", "Product: Laptop Dell, Price: 5999.99")
	logger.Log("ERROR", "Failed to extract from page")
	logger.Close()

	content, err := os.ReadFile(logger.FilePath())
	require.NoError(t, err)

	text := string(content)
	assert.Contains(t, text, "[FIRECRAWL_START]")
	assert.Contains(t, text, "[FIRECRAWL_PROGRESS]")
	assert.Contains(t, text, "[FIRECRAWL_COMPLETE]")
	assert.Contains(t, text, "[AI_REQUEST]")
	assert.Contains(t, text, "[PRODUCT_FOUND]")
	assert.Contains(t, text, "[ERROR]")

	lines := strings.Split(strings.TrimSpace(text), "\n")
	assert.Len(t, lines, 6)
	for _, line := range lines {
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

	logger.Log("FIRECRAWL_START", "test entry")
	logger.Close()

	content, err := os.ReadFile(logger.FilePath())
	require.NoError(t, err)
	assert.Contains(t, string(content), "[FIRECRAWL_START]")
}
```

- [ ] **Step 2: Update logger.go docstring**

In `backend/internal/crawler/logger.go`, update the comment on the `Log` method (line 48-50):

Replace:
```go
// Log writes a structured log entry with timestamp and level to both the log
// file and stdout.
// Level should be one of: FETCH, PARSE, AI_REQUEST, AI_RESPONSE, NAVIGATE,
// PRODUCT_FOUND, VALIDATION, ERROR.
```

With:
```go
// Log writes a structured log entry with timestamp and level to both the log
// file and stdout.
// Level should be one of: FIRECRAWL_START, FIRECRAWL_PROGRESS, FIRECRAWL_COMPLETE,
// AI_REQUEST, AI_RESPONSE, PRODUCT_FOUND, VALIDATION, ERROR.
```

- [ ] **Step 3: Run tests to verify they pass**

```bash
cd backend && go test ./internal/crawler/ -run TestCrawlLogger -v
```

Expected: All 3 tests PASS.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/crawler/logger.go backend/internal/crawler/logger_test.go
git commit -m "feat: update logger with Firecrawl log levels"
```

---

### Task 6: Delete fetcher.go, sitemap.go and their tests

**Files:**
- Delete: `backend/internal/crawler/fetcher.go`
- Delete: `backend/internal/crawler/fetcher_test.go`
- Delete: `backend/internal/crawler/sitemap.go`
- Delete: `backend/internal/crawler/sitemap_test.go`

- [ ] **Step 1: Delete the files**

```bash
cd backend && rm internal/crawler/fetcher.go internal/crawler/fetcher_test.go internal/crawler/sitemap.go internal/crawler/sitemap_test.go
```

- [ ] **Step 2: Verify the remaining code compiles (it won't yet — crawler.go still references Fetcher)**

```bash
cd backend && go build ./internal/crawler/ 2>&1 || true
```

Expected: Compilation errors referencing `Fetcher`, `FetchSitemapURLs`, `SampleURLs`, `ExtractLinks`, `ExtractProducts`. This is expected — Task 7 rewrites `crawler.go`.

- [ ] **Step 3: Commit the deletions**

```bash
git add -u backend/internal/crawler/fetcher.go backend/internal/crawler/fetcher_test.go backend/internal/crawler/sitemap.go backend/internal/crawler/sitemap_test.go
git commit -m "refactor: remove fetcher and sitemap code (replaced by Firecrawl)"
```

---

### Task 7: Rewrite crawler orchestrator

**Files:**
- Rewrite: `backend/internal/crawler/crawler.go`
- Rewrite: `backend/internal/crawler/crawler_test.go`

- [ ] **Step 1: Write the failing integration tests**

Replace the full contents of `backend/internal/crawler/crawler_test.go` with:

```go
package crawler_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/jzy/howmuchyousay/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock FirecrawlCrawler ---

type mockFirecrawlCrawler struct {
	crawlSiteFn func(ctx context.Context, siteURL string, cfg crawler.CrawlConfig) ([]crawler.PageResult, error)
}

func (m *mockFirecrawlCrawler) CrawlSite(ctx context.Context, siteURL string, cfg crawler.CrawlConfig) ([]crawler.PageResult, error) {
	if m.crawlSiteFn != nil {
		return m.crawlSiteFn(ctx, siteURL, cfg)
	}
	return nil, nil
}

// --- Mock AI Client ---

type mockAIClient struct {
	extractProductsFn func(ctx context.Context, markdown string, pageURL string) ([]crawler.RawProduct, int, error)
}

func (m *mockAIClient) ExtractProducts(ctx context.Context, markdown string, pageURL string) ([]crawler.RawProduct, int, error) {
	if m.extractProductsFn != nil {
		return m.extractProductsFn(ctx, markdown, pageURL)
	}
	return nil, 0, nil
}

// --- Test DB setup ---

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

func TestCrawler_RunWithFirecrawl(t *testing.T) {
	pool := setupTestDB(t)
	logDir := t.TempDir()

	mockFC := &mockFirecrawlCrawler{
		crawlSiteFn: func(ctx context.Context, siteURL string, cfg crawler.CrawlConfig) ([]crawler.PageResult, error) {
			return []crawler.PageResult{
				{
					URL:      "https://shop.com/products/laptop",
					Markdown: "# Laptop Dell XPS 15\n\nPrice: 5999.99 PLN\n\nAdd to cart",
					Metadata: map[string]string{
						"title":   "Laptop Dell XPS 15 - Shop",
						"ogImage": "https://img.com/dell.jpg",
					},
				},
				{
					URL:      "https://shop.com/products/iphone",
					Markdown: "# iPhone 15 Pro\n\nPrice: 5499 PLN\n\nBuy now",
					Metadata: map[string]string{
						"title":   "iPhone 15 Pro - Shop",
						"ogImage": "https://img.com/iphone.jpg",
					},
				},
			}, nil
		},
	}

	// AI extracts products from markdown
	callCount := 0
	mockAI := &mockAIClient{
		extractProductsFn: func(ctx context.Context, markdown string, pageURL string) ([]crawler.RawProduct, int, error) {
			callCount++
			if callCount == 1 {
				return []crawler.RawProduct{
					{Name: "Laptop Dell XPS 15", Price: 5999.99, ImageURL: "https://img.com/dell.jpg"},
				}, 200, nil
			}
			return []crawler.RawProduct{
				{Name: "iPhone 15 Pro", Price: 5499.00, ImageURL: "https://img.com/iphone.jpg"},
			}, 200, nil
		},
	}

	c := crawler.New(
		mockFC,
		mockAI,
		store.NewShopStore(pool),
		store.NewCrawlStore(pool),
		store.NewProductStore(pool),
		logDir,
	)

	cfg := crawler.CrawlConfig{
		URL:         "https://shop.com",
		Timeout:     30 * time.Second,
		MinProducts: 1,
		Verbose:     false,
	}

	result, err := c.Run(context.Background(), cfg, nil)
	require.NoError(t, err)

	assert.Equal(t, 2, result.ProductsFound)
	assert.Equal(t, 2, result.PagesVisited)
	assert.Equal(t, 2, result.AIRequestsCount)
	assert.NotEmpty(t, result.CrawlID)
	assert.NotEmpty(t, result.ShopID)
}

func TestCrawler_RunWithMetadataExtraction(t *testing.T) {
	pool := setupTestDB(t)
	logDir := t.TempDir()

	// Page with og:type=product metadata — no AI needed
	mockFC := &mockFirecrawlCrawler{
		crawlSiteFn: func(ctx context.Context, siteURL string, cfg crawler.CrawlConfig) ([]crawler.PageResult, error) {
			return []crawler.PageResult{
				{
					URL:      "https://shop.com/product/phone",
					Markdown: "# iPhone 15 Pro\n\nBest phone ever",
					Metadata: map[string]string{
						"ogTitle":              "iPhone 15 Pro",
						"ogImage":             "https://img.com/iphone.jpg",
						"og:type":             "product",
						"product:price:amount": "5499.00",
					},
				},
			}, nil
		},
	}

	mockAI := &mockAIClient{}

	c := crawler.New(
		mockFC,
		mockAI,
		store.NewShopStore(pool),
		store.NewCrawlStore(pool),
		store.NewProductStore(pool),
		logDir,
	)

	cfg := crawler.CrawlConfig{
		URL:         "https://shop.com",
		Timeout:     30 * time.Second,
		MinProducts: 1,
	}

	result, err := c.Run(context.Background(), cfg, nil)
	require.NoError(t, err)

	assert.Equal(t, 1, result.ProductsFound)
	assert.Equal(t, 0, result.AIRequestsCount) // No AI needed
}

func TestCrawler_RunFirecrawlError(t *testing.T) {
	pool := setupTestDB(t)
	logDir := t.TempDir()

	mockFC := &mockFirecrawlCrawler{
		crawlSiteFn: func(ctx context.Context, siteURL string, cfg crawler.CrawlConfig) ([]crawler.PageResult, error) {
			return nil, fmt.Errorf("firecrawl API error: 401 unauthorized")
		},
	}

	mockAI := &mockAIClient{}

	c := crawler.New(
		mockFC,
		mockAI,
		store.NewShopStore(pool),
		store.NewCrawlStore(pool),
		store.NewProductStore(pool),
		logDir,
	)

	cfg := crawler.CrawlConfig{
		URL:         "https://shop.com",
		Timeout:     10 * time.Second,
		MinProducts: 1,
	}

	result, err := c.Run(context.Background(), cfg, nil)
	require.NoError(t, err) // Crawl itself doesn't error — it marks the crawl as failed

	assert.Equal(t, 0, result.ProductsFound)
}

func TestCrawler_RunSavesToDatabase(t *testing.T) {
	pool := setupTestDB(t)
	logDir := t.TempDir()

	mockFC := &mockFirecrawlCrawler{
		crawlSiteFn: func(ctx context.Context, siteURL string, cfg crawler.CrawlConfig) ([]crawler.PageResult, error) {
			return []crawler.PageResult{
				{
					URL:      "https://shop.com/product/1",
					Markdown: "# DB Test Product\n\nPrice: 99.99 PLN",
					Metadata: map[string]string{
						"ogTitle":              "DB Test Product",
						"ogImage":             "https://img.com/db.jpg",
						"og:type":             "product",
						"product:price:amount": "99.99",
					},
				},
			}, nil
		},
	}

	mockAI := &mockAIClient{}

	shopStore := store.NewShopStore(pool)
	crawlStore := store.NewCrawlStore(pool)
	productStore := store.NewProductStore(pool)

	c := crawler.New(
		mockFC,
		mockAI,
		shopStore,
		crawlStore,
		productStore,
		logDir,
	)

	cfg := crawler.CrawlConfig{
		URL:         "https://shop.com",
		Timeout:     10 * time.Second,
		MinProducts: 1,
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

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd backend && go test ./internal/crawler/ -run "TestCrawler_" -v 2>&1 | head -30
```

Expected: FAIL — `crawler.New` constructor signature doesn't match (still expects `Fetcher`).

- [ ] **Step 3: Rewrite crawler.go**

Replace the full contents of `backend/internal/crawler/crawler.go` with:

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
type ProgressFunc func(event string, message string)

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
	fc           FirecrawlCrawler
	ai           AIClient
	shopStore    *store.ShopStore
	crawlStore   *store.CrawlStore
	productStore *store.ProductStore
	logDir       string
}

// New creates a new Crawler with all dependencies.
func New(
	fc FirecrawlCrawler,
	ai AIClient,
	shopStore *store.ShopStore,
	crawlStore *store.CrawlStore,
	productStore *store.ProductStore,
	logDir string,
) *Crawler {
	return &Crawler{
		fc:           fc,
		ai:           ai,
		shopStore:    shopStore,
		crawlStore:   crawlStore,
		productStore: productStore,
		logDir:       logDir,
	}
}

// Run executes the full crawl pipeline for the given config.
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

	// 2. Create crawl record
	crawl, err := c.crawlStore.Create(ctx, shop.ID, sessionID, "")
	if err != nil {
		return nil, fmt.Errorf("creating crawl record: %w", err)
	}

	logger, err := NewCrawlLogger(c.logDir, crawl.ID)
	if err != nil {
		return nil, fmt.Errorf("creating crawl logger: %w", err)
	}
	defer logger.Close()

	if err := c.crawlStore.UpdateLogPath(ctx, crawl.ID, logger.FilePath()); err != nil {
		return nil, fmt.Errorf("updating crawl log path: %w", err)
	}

	if err := c.crawlStore.UpdateStatus(ctx, crawl.ID, models.CrawlStatusInProgress); err != nil {
		return nil, fmt.Errorf("updating crawl status: %w", err)
	}

	logger.Log("FIRECRAWL_START", fmt.Sprintf("Starting Firecrawl job for %s", cfg.URL))
	report("start", fmt.Sprintf("Starting crawl for %s", cfg.URL))

	// 3. Run Firecrawl crawl
	pages, err := c.fc.CrawlSite(ctx, cfg.URL, cfg)
	if err != nil {
		errMsg := fmt.Sprintf("Firecrawl error: %v", err)
		logger.Log("ERROR", errMsg)
		c.crawlStore.Finish(ctx, crawl.ID, models.CrawlStatusFailed, 0, 0, 0, &errMsg)
		return &CrawlResult{
			CrawlID:     crawl.ID,
			ShopID:      shop.ID,
			Duration:    time.Since(start),
			LogFilePath: logger.FilePath(),
		}, nil
	}

	logger.Log("FIRECRAWL_COMPLETE", fmt.Sprintf("Firecrawl returned %d pages", len(pages)))
	report("firecrawl_complete", fmt.Sprintf("Firecrawl returned %d pages", len(pages)))

	if len(pages) == 0 {
		errMsg := "Firecrawl returned 0 pages"
		logger.Log("ERROR", errMsg)
		c.crawlStore.Finish(ctx, crawl.ID, models.CrawlStatusFailed, 0, 0, 0, &errMsg)
		return &CrawlResult{
			CrawlID:     crawl.ID,
			ShopID:      shop.ID,
			Duration:    time.Since(start),
			LogFilePath: logger.FilePath(),
		}, nil
	}

	// 4. Extract products from each page
	var allProducts []RawProduct
	pagesVisited := 0
	aiRequestsCount := 0

	for i, page := range pages {
		if ctx.Err() != nil {
			logger.Log("ERROR", "Timeout reached during product extraction")
			break
		}

		if !PageHasContent(page) {
			continue
		}
		pagesVisited++

		report("extract", fmt.Sprintf("Processing page %d/%d: %s", i+1, len(pages), page.URL))

		// Try metadata-based extraction first (OG product data)
		products := ExtractProductsFromPage(page)
		if len(products) > 0 {
			logger.Log("PRODUCT_FOUND", fmt.Sprintf("Found %d products via metadata on %s", len(products), page.URL))
		}

		// If no products from metadata and page has product signals, use AI
		if len(products) == 0 && c.ai != nil && HasProductSignals(page) {
			logger.Log("AI_REQUEST", fmt.Sprintf("Sending markdown to AI for %s", page.URL))
			aiProducts, tokensUsed, err := c.ai.ExtractProducts(ctx, page.Markdown, page.URL)
			if err != nil {
				logger.Log("ERROR", fmt.Sprintf("AI extraction failed for %s: %v", page.URL, err))
			} else {
				aiRequestsCount++
				logger.Log("AI_RESPONSE", fmt.Sprintf("AI found %d products, used %d tokens", len(aiProducts), tokensUsed))
				products = aiProducts
			}
		}

		for _, p := range products {
			logger.Log("PRODUCT_FOUND", fmt.Sprintf("Product: %s, Price: %.2f", p.Name, p.Price))
		}
		allProducts = append(allProducts, products...)
		report("products", fmt.Sprintf("Total products so far: %d", len(allProducts)))
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
		msg := fmt.Sprintf("found only %d products (minimum: %d)", savedCount, cfg.MinProducts)
		errMsg = &msg
		logger.Log("ERROR", msg)
	}

	if err := c.crawlStore.Finish(ctx, crawl.ID, status, savedCount, pagesVisited, aiRequestsCount, errMsg); err != nil {
		logger.Log("ERROR", fmt.Sprintf("Failed to finish crawl record: %v", err))
	}

	logger.Log("FIRECRAWL_COMPLETE", fmt.Sprintf("Crawl complete: %d products, %d pages, %d AI requests, %v", savedCount, pagesVisited, aiRequestsCount, duration))
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
func normalizeShopURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	normalized := fmt.Sprintf("%s://%s", parsed.Scheme, strings.ToLower(parsed.Host))
	return normalized
}
```

- [ ] **Step 4: Verify compilation**

```bash
cd backend && go build ./internal/crawler/
```

Expected: Compiles without errors.

- [ ] **Step 5: Run the integration tests (requires test database)**

```bash
cd backend && go test ./internal/crawler/ -run "TestCrawler_" -v
```

Expected: All 4 tests PASS. (These require the test PostgreSQL database to be running.)

- [ ] **Step 6: Run ALL crawler tests**

```bash
cd backend && go test ./internal/crawler/ -v
```

Expected: All tests PASS (firecrawl_client, ai_client, extractor, logger, validator, crawler).

- [ ] **Step 7: Commit**

```bash
git add backend/internal/crawler/crawler.go backend/internal/crawler/crawler_test.go
git commit -m "feat: rewrite crawler orchestrator to use Firecrawl instead of BFS"
```

---

### Task 8: Update CLI entry point

**Files:**
- Modify: `backend/cmd/crawler/main.go`

- [ ] **Step 1: Rewrite main.go**

Replace the full contents of `backend/cmd/crawler/main.go` with:

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

	if cfg.FirecrawlAPIKey == "" {
		fmt.Fprintln(os.Stderr, "Error: FIRECRAWL_API_KEY must be set")
		os.Exit(1)
	}

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
	firecrawlClient := crawler.NewFirecrawlClient(cfg.FirecrawlAPIKey, cfg.FirecrawlAPIURL)

	var aiClient crawler.AIClient
	if cfg.OpenAIAPIKey != "" {
		aiClient = crawler.NewOpenAIClient(cfg.OpenAIAPIKey, cfg.OpenAIModel, "")
	}

	c := crawler.New(
		firecrawlClient,
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
		fmt.Printf("  Firecrawl API: %s\n", cfg.FirecrawlAPIURL)
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
	return "../migrations"
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd backend && go build ./cmd/crawler/
```

Expected: Compiles without errors.

- [ ] **Step 3: Commit**

```bash
git add backend/cmd/crawler/main.go
git commit -m "feat: update CLI to use Firecrawl client instead of HTTPFetcher"
```

---

### Task 9: Remove unused dependency and clean up go.mod

**Files:**
- Modify: `backend/go.mod`
- Modify: `backend/go.sum`

- [ ] **Step 1: Tidy the module**

```bash
cd backend && go mod tidy
```

This should:
- Add `github.com/mendableai/firecrawl-go/v2` to `go.mod` (if not already there from Task 1)
- Remove `golang.org/x/time` if it's no longer used by any code (it may still be an indirect dependency)

- [ ] **Step 2: Verify everything still compiles**

```bash
cd backend && go build ./...
```

Expected: Compiles without errors.

- [ ] **Step 3: Run full test suite**

```bash
cd backend && go test ./... -v
```

Expected: All tests pass.

- [ ] **Step 4: Commit**

```bash
git add backend/go.mod backend/go.sum
git commit -m "chore: tidy go.mod after Firecrawl migration"
```

---

### Task 10: Final verification

- [ ] **Step 1: Run full build**

```bash
cd backend && go build ./...
```

Expected: Clean compilation.

- [ ] **Step 2: Run full test suite**

```bash
cd backend && go test ./... -v -count=1
```

Expected: All tests pass. No references to deleted `Fetcher`, `ExtractLinks`, `SampleURLs`, `FetchSitemapURLs`.

- [ ] **Step 3: Check for any remaining references to deleted code**

```bash
cd backend && grep -r "Fetcher\|FetchSitemapURLs\|SampleURLs\|ExtractLinks\|CheckRobotsTxt\|HTTPFetcher\|NewHTTPFetcher" --include="*.go" internal/ cmd/ || echo "No stale references found"
```

Expected: "No stale references found" (or only interface references in test mocks, which we've already updated).

- [ ] **Step 4: Verify .env.example has all required vars**

```bash
cat ../.env.example
```

Expected: Contains `FIRECRAWL_API_KEY`, `FIRECRAWL_API_URL`, `FIRECRAWL_MAX_DEPTH`.
