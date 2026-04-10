# AI-Driven Crawl Loop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Firecrawl-crawl-then-extract pipeline with an AI-driven orchestrator loop where an OpenAI model navigates shop pages using Firecrawl as tools.

**Architecture:** An OpenAI function-calling loop drives crawl navigation. The model has 5 tools: `discover_links` (Firecrawl ScrapeURL with `links` format), `extract_product` (Firecrawl ScrapeURL with `json` format + LLM extraction schema), `save_product` (validate + persist to DB), `get_status` (return progress), and `done` (end loop). Go code executes tool calls and manages conversation state.

**Tech Stack:** Go 1.26.2, `openai-go/v3` (v3.31.0), `firecrawl-go/v2` (v2.4.0), PostgreSQL via pgx/v5, testify for assertions.

**Spec:** `docs/superpowers/specs/2026-04-10-ai-driven-crawl-loop-design.md`

**Test note:** Tests in `internal/crawler` and `internal/store` must be run separately (`go test ./internal/crawler/` and `go test ./internal/store/`) due to pre-existing cross-package test pollution with `go test ./...`.

---

## File Structure

```
backend/internal/crawler/
├── firecrawl_scraper.go      (NEW — FirecrawlScraper interface + FirecrawlClient with DiscoverLinks/ExtractProduct)
├── firecrawl_scraper_test.go  (NEW — unit tests for scraper)
├── orchestrator.go            (NEW — Orchestrator interface + OpenAI tool-calling loop)
├── orchestrator_test.go       (NEW — unit tests for orchestrator with mock OpenAI server)
├── tool_handlers.go           (NEW — ToolHandlers struct, save_product/get_status/done logic)
├── tool_handlers_test.go      (NEW — unit tests for tool handlers)
├── crawler.go                 (REWRITE — Crawler.Run uses orchestrator instead of page-iterate-extract)
├── crawler_test.go            (REWRITE — integration tests with new mocks)
├── validator.go               (UNCHANGED)
├── validator_test.go          (UNCHANGED)
├── logger.go                  (UNCHANGED — new log events used via existing Log method)
├── logger_test.go             (UNCHANGED)
├── firecrawl_client.go        (DELETE)
├── firecrawl_client_test.go   (DELETE)
├── ai_client.go               (DELETE)
├── ai_client_test.go          (DELETE)
├── extractor.go               (DELETE)
├── extractor_test.go          (DELETE)

backend/internal/config/
├── config.go                  (MODIFY — replace FirecrawlMaxDepth with FirecrawlMaxScrapes + CrawlTimeout)

backend/cmd/crawler/
├── main.go                    (MODIFY — wire up new types, update CLI flags)

.env.example                   (MODIFY — update env vars)
```

---

### Task 1: Update Config and .env.example

**Files:**
- Modify: `backend/internal/config/config.go`
- Modify: `.env.example`

- [ ] **Step 1: Update config struct and Load function**

Replace `FirecrawlMaxDepth` with `FirecrawlMaxScrapes` and add `CrawlTimeout` in `backend/internal/config/config.go`:

```go
type Config struct {
	DatabaseURL         string
	OpenAIAPIKey        string
	OpenAIModel         string
	LogDir              string
	ServerPort          string
	FirecrawlAPIKey     string
	FirecrawlAPIURL     string
	FirecrawlMaxScrapes int
	CrawlTimeout        int
}

func Load() *Config {
	return &Config{
		DatabaseURL:         getEnv("DATABASE_URL", "postgres://hmys:hmys_dev@localhost:5432/howmuchyousay?sslmode=disable"),
		OpenAIAPIKey:        getEnv("OPENAI_API_KEY", ""),
		OpenAIModel:         getEnv("OPENAI_MODEL", "gpt-5-mini"),
		LogDir:              getEnv("LOG_DIR", "./logs"),
		ServerPort:          getEnv("SERVER_PORT", "8080"),
		FirecrawlAPIKey:     getEnv("FIRECRAWL_API_KEY", ""),
		FirecrawlAPIURL:     getEnv("FIRECRAWL_API_URL", "https://api.firecrawl.dev"),
		FirecrawlMaxScrapes: getEnvInt("FIRECRAWL_MAX_SCRAPES", 50),
		CrawlTimeout:        getEnvInt("CRAWL_TIMEOUT", 300),
	}
}
```

- [ ] **Step 2: Update .env.example**

Replace contents of `.env.example`:

```
DATABASE_URL=postgres://hmys:hmys_dev@localhost:5432/howmuchyousay?sslmode=disable
TEST_DATABASE_URL=postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable
OPENAI_API_KEY=sk-your-key-here
OPENAI_MODEL=gpt-5-mini
LOG_DIR=./logs
FIRECRAWL_API_KEY=fc-your-key-here
FIRECRAWL_API_URL=https://api.firecrawl.dev
FIRECRAWL_MAX_SCRAPES=50
CRAWL_TIMEOUT=300
```

- [ ] **Step 3: Verify build compiles**

Run: `go build ./...` from `backend/`
Expected: Build will fail because `crawler.go` and `cmd/crawler/main.go` still reference `FirecrawlMaxDepth` and `MaxDepth`. That's expected — we'll fix those files in later tasks.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/config/config.go .env.example
git commit -m "config: replace FirecrawlMaxDepth with FirecrawlMaxScrapes and CrawlTimeout"
```

---

### Task 2: Create FirecrawlScraper Interface and Implementation

**Files:**
- Create: `backend/internal/crawler/firecrawl_scraper.go`
- Create: `backend/internal/crawler/firecrawl_scraper_test.go`

- [ ] **Step 1: Write the failing tests**

Create `backend/internal/crawler/firecrawl_scraper_test.go`:

```go
package crawler

import (
	"context"
	"testing"

	firecrawl "github.com/mendableai/firecrawl-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFirecrawlScraperInterface(t *testing.T) {
	t.Run("FirecrawlClient implements FirecrawlScraper", func(t *testing.T) {
		var _ FirecrawlScraper = &FirecrawlClient{}
	})
}

func TestConvertLinksResponse(t *testing.T) {
	strPtr := func(s string) *string { return &s }

	t.Run("extracts links and title from document", func(t *testing.T) {
		doc := &firecrawl.FirecrawlDocument{
			Links: []string{
				"https://shop.com/product/1",
				"https://shop.com/category/phones",
				"https://shop.com/page/2",
			},
			Metadata: &firecrawl.FirecrawlDocumentMetadata{
				Title:     strPtr("Electronics Store"),
				SourceURL: strPtr("https://shop.com"),
			},
		}

		result := convertLinksResponse(doc)

		assert.Equal(t, "Electronics Store", result.PageTitle)
		assert.Equal(t, "https://shop.com", result.PageURL)
		require.Len(t, result.Links, 3)
		assert.Equal(t, "https://shop.com/product/1", result.Links[0])
		assert.Equal(t, "https://shop.com/category/phones", result.Links[1])
	})

	t.Run("handles nil metadata", func(t *testing.T) {
		doc := &firecrawl.FirecrawlDocument{
			Links:    []string{"https://shop.com/a"},
			Metadata: nil,
		}

		result := convertLinksResponse(doc)

		assert.Equal(t, "", result.PageTitle)
		assert.Equal(t, "", result.PageURL)
		assert.Len(t, result.Links, 1)
	})

	t.Run("handles empty links", func(t *testing.T) {
		doc := &firecrawl.FirecrawlDocument{
			Links:    []string{},
			Metadata: &firecrawl.FirecrawlDocumentMetadata{Title: strPtr("Empty Page")},
		}

		result := convertLinksResponse(doc)

		assert.Equal(t, "Empty Page", result.PageTitle)
		assert.Empty(t, result.Links)
	})

	t.Run("handles nil document", func(t *testing.T) {
		result := convertLinksResponse(nil)
		assert.Equal(t, "", result.PageTitle)
		assert.Empty(t, result.Links)
	})
}

func TestConvertProductResponse(t *testing.T) {
	t.Run("extracts product from JSON field", func(t *testing.T) {
		doc := &firecrawl.FirecrawlDocument{
			JSON: map[string]any{
				"name":      "iPhone 15 Pro",
				"price":     float64(999.99),
				"image_url": "https://img.com/iphone.jpg",
			},
		}

		result := convertProductResponse(doc)

		assert.Equal(t, "iPhone 15 Pro", result.Name)
		assert.Equal(t, 999.99, result.Price)
		assert.Equal(t, "https://img.com/iphone.jpg", result.ImageURL)
		assert.True(t, result.Found)
	})

	t.Run("handles missing optional image_url", func(t *testing.T) {
		doc := &firecrawl.FirecrawlDocument{
			JSON: map[string]any{
				"name":  "Laptop Dell",
				"price": float64(1499.00),
			},
		}

		result := convertProductResponse(doc)

		assert.Equal(t, "Laptop Dell", result.Name)
		assert.Equal(t, 1499.00, result.Price)
		assert.Equal(t, "", result.ImageURL)
		assert.True(t, result.Found)
	})

	t.Run("returns not found for nil JSON", func(t *testing.T) {
		doc := &firecrawl.FirecrawlDocument{
			JSON: nil,
		}

		result := convertProductResponse(doc)
		assert.False(t, result.Found)
	})

	t.Run("returns not found for empty JSON", func(t *testing.T) {
		doc := &firecrawl.FirecrawlDocument{
			JSON: map[string]any{},
		}

		result := convertProductResponse(doc)
		assert.False(t, result.Found)
	})

	t.Run("returns not found for nil document", func(t *testing.T) {
		result := convertProductResponse(nil)
		assert.False(t, result.Found)
	})

	t.Run("handles price as int", func(t *testing.T) {
		doc := &firecrawl.FirecrawlDocument{
			JSON: map[string]any{
				"name":  "Test Product",
				"price": float64(100),
			},
		}

		result := convertProductResponse(doc)
		assert.Equal(t, 100.0, result.Price)
		assert.True(t, result.Found)
	})
}

func TestNewFirecrawlClientScraper(t *testing.T) {
	t.Run("creates client with valid credentials", func(t *testing.T) {
		client, err := NewFirecrawlClient("test-api-key", "https://api.firecrawl.dev")
		require.NoError(t, err)
		require.NotNil(t, client)
	})

	t.Run("returns error with empty API key", func(t *testing.T) {
		t.Setenv("FIRECRAWL_API_KEY", "")
		client, err := NewFirecrawlClient("", "https://api.firecrawl.dev")
		assert.Error(t, err)
		assert.Nil(t, client)
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/crawler/ -run "TestFirecrawlScraperInterface|TestConvertLinksResponse|TestConvertProductResponse|TestNewFirecrawlClientScraper" -v` from `backend/`
Expected: FAIL — types `FirecrawlScraper`, `LinkDiscoveryResult`, `ProductExtractionResult`, `convertLinksResponse`, `convertProductResponse` not defined.

- [ ] **Step 3: Write the implementation**

Create `backend/internal/crawler/firecrawl_scraper.go`:

```go
package crawler

import (
	"context"
	"fmt"

	firecrawl "github.com/mendableai/firecrawl-go/v2"
)

// LinkDiscoveryResult contains the links and metadata from a page exploration.
type LinkDiscoveryResult struct {
	PageURL   string
	PageTitle string
	Links     []string
}

// ProductExtractionResult contains structured product data extracted by Firecrawl's LLM.
type ProductExtractionResult struct {
	Name     string
	Price    float64
	ImageURL string
	Found    bool
}

// FirecrawlScraper defines the interface for single-page scraping via Firecrawl.
type FirecrawlScraper interface {
	DiscoverLinks(ctx context.Context, url string) (*LinkDiscoveryResult, error)
	ExtractProduct(ctx context.Context, url string) (*ProductExtractionResult, error)
}

// FirecrawlClient implements FirecrawlScraper using the firecrawl-go SDK.
type FirecrawlClient struct {
	app *firecrawl.FirecrawlApp
}

// NewFirecrawlClient creates a new FirecrawlClient with the given API key and URL.
func NewFirecrawlClient(apiKey, apiURL string) (*FirecrawlClient, error) {
	app, err := firecrawl.NewFirecrawlApp(apiKey, apiURL)
	if err != nil {
		return nil, fmt.Errorf("creating firecrawl app: %w", err)
	}
	return &FirecrawlClient{app: app}, nil
}

// DiscoverLinks scrapes a page and returns all links found on it.
func (fc *FirecrawlClient) DiscoverLinks(ctx context.Context, url string) (*LinkDiscoveryResult, error) {
	onlyMainContent := true
	params := &firecrawl.ScrapeParams{
		Formats:         []string{"links"},
		OnlyMainContent: &onlyMainContent,
	}

	doc, err := fc.app.ScrapeURL(url, params)
	if err != nil {
		return nil, fmt.Errorf("firecrawl scrape (links): %w", err)
	}

	return convertLinksResponse(doc), nil
}

// ExtractProduct scrapes a product page and extracts structured data via Firecrawl's LLM.
func (fc *FirecrawlClient) ExtractProduct(ctx context.Context, url string) (*ProductExtractionResult, error) {
	prompt := "Extract the product name, price (as a number), and main product image URL from this product page."
	params := &firecrawl.ScrapeParams{
		Formats: []string{"json"},
		JsonOptions: &firecrawl.JsonOptions{
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":      map[string]any{"type": "string"},
					"price":     map[string]any{"type": "number"},
					"image_url": map[string]any{"type": "string"},
				},
				"required": []string{"name", "price"},
			},
			Prompt: &prompt,
		},
	}

	doc, err := fc.app.ScrapeURL(url, params)
	if err != nil {
		return nil, fmt.Errorf("firecrawl scrape (json): %w", err)
	}

	result := convertProductResponse(doc)
	return result, nil
}

// convertLinksResponse transforms a Firecrawl document into a LinkDiscoveryResult.
func convertLinksResponse(doc *firecrawl.FirecrawlDocument) *LinkDiscoveryResult {
	if doc == nil {
		return &LinkDiscoveryResult{}
	}

	result := &LinkDiscoveryResult{
		Links: doc.Links,
	}

	if doc.Metadata != nil {
		if doc.Metadata.Title != nil {
			result.PageTitle = *doc.Metadata.Title
		}
		if doc.Metadata.SourceURL != nil {
			result.PageURL = *doc.Metadata.SourceURL
		} else if doc.Metadata.URL != nil {
			result.PageURL = *doc.Metadata.URL
		}
	}

	return result
}

// convertProductResponse transforms a Firecrawl document into a ProductExtractionResult.
func convertProductResponse(doc *firecrawl.FirecrawlDocument) *ProductExtractionResult {
	if doc == nil || doc.JSON == nil || len(doc.JSON) == 0 {
		return &ProductExtractionResult{Found: false}
	}

	name, _ := doc.JSON["name"].(string)
	price, _ := doc.JSON["price"].(float64)
	imageURL, _ := doc.JSON["image_url"].(string)

	if name == "" || price == 0 {
		return &ProductExtractionResult{Found: false}
	}

	return &ProductExtractionResult{
		Name:     name,
		Price:    price,
		ImageURL: imageURL,
		Found:    true,
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/crawler/ -run "TestFirecrawlScraperInterface|TestConvertLinksResponse|TestConvertProductResponse|TestNewFirecrawlClientScraper" -v` from `backend/`
Expected: PASS — note that `firecrawl_client.go` still exists and will cause a duplicate `FirecrawlClient` and `NewFirecrawlClient` compile error. Delete the old files first:

```bash
rm backend/internal/crawler/firecrawl_client.go backend/internal/crawler/firecrawl_client_test.go
```

Then run the tests again.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/crawler/firecrawl_scraper.go backend/internal/crawler/firecrawl_scraper_test.go
git rm backend/internal/crawler/firecrawl_client.go backend/internal/crawler/firecrawl_client_test.go
git commit -m "feat: add FirecrawlScraper with DiscoverLinks and ExtractProduct"
```

---

### Task 3: Create Tool Handlers

**Files:**
- Create: `backend/internal/crawler/tool_handlers.go`
- Create: `backend/internal/crawler/tool_handlers_test.go`

- [ ] **Step 1: Write the failing tests**

Create `backend/internal/crawler/tool_handlers_test.go`:

```go
package crawler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolHandlers_SaveProduct(t *testing.T) {
	t.Run("saves valid product and returns progress", func(t *testing.T) {
		h := NewToolHandlers(20)

		result := h.SaveProduct(RawProduct{
			Name:      "iPhone 15 Pro",
			Price:     999.99,
			ImageURL:  "https://img.com/iphone.jpg",
			SourceURL: "https://shop.com/product/iphone",
		})

		assert.Contains(t, result, "Saved")
		assert.Contains(t, result, "iPhone 15 Pro")
		assert.Contains(t, result, "1/20")
		assert.Len(t, h.SavedProducts(), 1)
	})

	t.Run("rejects product with empty name", func(t *testing.T) {
		h := NewToolHandlers(20)

		result := h.SaveProduct(RawProduct{
			Name:      "",
			Price:     100.00,
			SourceURL: "https://shop.com/product/1",
		})

		assert.Contains(t, result, "rejected")
		assert.Len(t, h.SavedProducts(), 0)
	})

	t.Run("rejects product with zero price", func(t *testing.T) {
		h := NewToolHandlers(20)

		result := h.SaveProduct(RawProduct{
			Name:      "Free Thing",
			Price:     0,
			SourceURL: "https://shop.com/product/1",
		})

		assert.Contains(t, result, "rejected")
		assert.Len(t, h.SavedProducts(), 0)
	})

	t.Run("rejects duplicate product by name", func(t *testing.T) {
		h := NewToolHandlers(20)

		h.SaveProduct(RawProduct{Name: "iPhone 15 Pro", Price: 999.99, SourceURL: "https://shop.com/a"})
		result := h.SaveProduct(RawProduct{Name: "iphone 15 pro", Price: 899.99, SourceURL: "https://shop.com/b"})

		assert.Contains(t, result, "duplicate")
		assert.Len(t, h.SavedProducts(), 1)
	})

	t.Run("rejects duplicate product by source URL", func(t *testing.T) {
		h := NewToolHandlers(20)

		h.SaveProduct(RawProduct{Name: "Product A", Price: 100, SourceURL: "https://shop.com/p/1"})
		result := h.SaveProduct(RawProduct{Name: "Product B", Price: 200, SourceURL: "https://shop.com/p/1"})

		assert.Contains(t, result, "duplicate")
		assert.Len(t, h.SavedProducts(), 1)
	})
}

func TestToolHandlers_GetStatus(t *testing.T) {
	t.Run("returns status with no products", func(t *testing.T) {
		h := NewToolHandlers(20)

		result := h.GetStatus()

		assert.Contains(t, result, "0/20")
	})

	t.Run("returns status with saved products", func(t *testing.T) {
		h := NewToolHandlers(20)
		h.SaveProduct(RawProduct{Name: "iPhone", Price: 999, SourceURL: "https://shop.com/1"})
		h.SaveProduct(RawProduct{Name: "Laptop", Price: 1499, SourceURL: "https://shop.com/2"})

		result := h.GetStatus()

		assert.Contains(t, result, "2/20")
		assert.Contains(t, result, "iPhone")
		assert.Contains(t, result, "Laptop")
	})
}

func TestToolHandlers_VisitedURL(t *testing.T) {
	t.Run("tracks visited URLs", func(t *testing.T) {
		h := NewToolHandlers(20)

		assert.False(t, h.IsVisited("https://shop.com/a"))
		h.MarkVisited("https://shop.com/a")
		assert.True(t, h.IsVisited("https://shop.com/a"))
		assert.False(t, h.IsVisited("https://shop.com/b"))
	})
}

func TestToolHandlers_ScrapeCount(t *testing.T) {
	t.Run("tracks scrape count", func(t *testing.T) {
		h := NewToolHandlers(20)

		assert.Equal(t, 0, h.ScrapeCount())
		h.IncrementScrapeCount()
		h.IncrementScrapeCount()
		assert.Equal(t, 2, h.ScrapeCount())
	})
}

func TestToolHandlers_HasReachedTarget(t *testing.T) {
	t.Run("returns false before target", func(t *testing.T) {
		h := NewToolHandlers(2)
		h.SaveProduct(RawProduct{Name: "A", Price: 10, SourceURL: "https://shop.com/1"})
		assert.False(t, h.HasReachedTarget())
	})

	t.Run("returns true at target", func(t *testing.T) {
		h := NewToolHandlers(2)
		h.SaveProduct(RawProduct{Name: "A", Price: 10, SourceURL: "https://shop.com/1"})
		h.SaveProduct(RawProduct{Name: "B", Price: 20, SourceURL: "https://shop.com/2"})
		assert.True(t, h.HasReachedTarget())
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/crawler/ -run "TestToolHandlers" -v` from `backend/`
Expected: FAIL — `NewToolHandlers`, `ToolHandlers` not defined.

- [ ] **Step 3: Write the implementation**

Create `backend/internal/crawler/tool_handlers.go`:

```go
package crawler

import (
	"fmt"
	"strings"
)

// ToolHandlers manages crawl state for the orchestrator loop: visited URLs,
// saved products, scrape count, and deduplication.
type ToolHandlers struct {
	minProducts   int
	savedProducts []RawProduct
	seenNames     map[string]bool
	seenURLs      map[string]bool
	visitedURLs   map[string]bool
	scrapeCount   int
}

// NewToolHandlers creates a ToolHandlers with the given product target.
func NewToolHandlers(minProducts int) *ToolHandlers {
	return &ToolHandlers{
		minProducts: minProducts,
		seenNames:   make(map[string]bool),
		seenURLs:    make(map[string]bool),
		visitedURLs: make(map[string]bool),
	}
}

// SaveProduct validates and saves a product. Returns a status message for the orchestrator.
func (h *ToolHandlers) SaveProduct(p RawProduct) string {
	// Validate using existing validator (single-item slice)
	valid, rejected := ValidateProducts([]RawProduct{p})
	if len(rejected) > 0 {
		return fmt.Sprintf("Product rejected: %s. Not counted. Progress: %d/%d products.",
			rejected[0].Reason, len(h.savedProducts), h.minProducts)
	}
	if len(valid) == 0 {
		return fmt.Sprintf("Product rejected: validation failed. Not counted. Progress: %d/%d products.",
			len(h.savedProducts), h.minProducts)
	}

	p = valid[0]

	// Check duplicate by source URL
	if p.SourceURL != "" && h.seenURLs[p.SourceURL] {
		return fmt.Sprintf("Duplicate product (same URL) skipped. Progress: %d/%d products.",
			len(h.savedProducts), h.minProducts)
	}

	// Check duplicate by normalized name
	normalized := NormalizeName(p.Name)
	if h.seenNames[normalized] {
		return fmt.Sprintf("Duplicate product (same name) skipped. Progress: %d/%d products.",
			len(h.savedProducts), h.minProducts)
	}

	h.savedProducts = append(h.savedProducts, p)
	h.seenNames[normalized] = true
	if p.SourceURL != "" {
		h.seenURLs[p.SourceURL] = true
	}

	return fmt.Sprintf("Saved \"%s\" ($%.2f). Progress: %d/%d products.\nSaved so far: %s",
		p.Name, p.Price, len(h.savedProducts), h.minProducts, h.productNameList())
}

// GetStatus returns the current crawl progress.
func (h *ToolHandlers) GetStatus() string {
	if len(h.savedProducts) == 0 {
		return fmt.Sprintf("Progress: 0/%d products. No products saved yet.", h.minProducts)
	}
	return fmt.Sprintf("Progress: %d/%d products.\nSaved so far: %s",
		len(h.savedProducts), h.minProducts, h.productNameList())
}

// SavedProducts returns all saved products.
func (h *ToolHandlers) SavedProducts() []RawProduct {
	return h.savedProducts
}

// IsVisited checks if a URL has already been scraped.
func (h *ToolHandlers) IsVisited(url string) bool {
	return h.visitedURLs[url]
}

// MarkVisited records a URL as already scraped.
func (h *ToolHandlers) MarkVisited(url string) {
	h.visitedURLs[url] = true
}

// ScrapeCount returns the number of Firecrawl scrape calls made.
func (h *ToolHandlers) ScrapeCount() int {
	return h.scrapeCount
}

// IncrementScrapeCount increments the scrape call counter.
func (h *ToolHandlers) IncrementScrapeCount() {
	h.scrapeCount++
}

// HasReachedTarget returns true if we've saved enough products.
func (h *ToolHandlers) HasReachedTarget() bool {
	return len(h.savedProducts) >= h.minProducts
}

// productNameList returns a comma-separated list of saved product names.
func (h *ToolHandlers) productNameList() string {
	names := make([]string, len(h.savedProducts))
	for i, p := range h.savedProducts {
		names[i] = p.Name
	}
	return strings.Join(names, ", ")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/crawler/ -run "TestToolHandlers" -v` from `backend/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/crawler/tool_handlers.go backend/internal/crawler/tool_handlers_test.go
git commit -m "feat: add ToolHandlers for crawl state management"
```

---

### Task 4: Create Orchestrator (OpenAI Tool-Calling Loop)

**Files:**
- Create: `backend/internal/crawler/orchestrator.go`
- Create: `backend/internal/crawler/orchestrator_test.go`

- [ ] **Step 1: Write the failing tests**

Create `backend/internal/crawler/orchestrator_test.go`:

```go
package crawler_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockFirecrawlScraper implements FirecrawlScraper for testing.
type mockFirecrawlScraper struct {
	discoverLinksFn   func(ctx context.Context, url string) (*crawler.LinkDiscoveryResult, error)
	extractProductFn  func(ctx context.Context, url string) (*crawler.ProductExtractionResult, error)
}

func (m *mockFirecrawlScraper) DiscoverLinks(ctx context.Context, url string) (*crawler.LinkDiscoveryResult, error) {
	if m.discoverLinksFn != nil {
		return m.discoverLinksFn(ctx, url)
	}
	return &crawler.LinkDiscoveryResult{}, nil
}

func (m *mockFirecrawlScraper) ExtractProduct(ctx context.Context, url string) (*crawler.ProductExtractionResult, error) {
	if m.extractProductFn != nil {
		return m.extractProductFn(ctx, url)
	}
	return &crawler.ProductExtractionResult{Found: false}, nil
}

// openAIToolCallResponse creates a mock OpenAI response with tool calls.
func openAIToolCallResponse(toolCalls []map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"id":     "chatcmpl-test",
		"object": "chat.completion",
		"model":  "gpt-5-mini",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":       "assistant",
					"content":    nil,
					"tool_calls": toolCalls,
				},
				"finish_reason": "tool_calls",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     100,
			"completion_tokens": 50,
			"total_tokens":      150,
		},
	}
}

// openAIStopResponse creates a mock OpenAI response that ends the conversation.
func openAIStopResponse(content string) map[string]interface{} {
	return map[string]interface{}{
		"id":     "chatcmpl-test",
		"object": "chat.completion",
		"model":  "gpt-5-mini",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     50,
			"completion_tokens": 20,
			"total_tokens":      70,
		},
	}
}

func TestOrchestrator_BasicFlow(t *testing.T) {
	// Simulate: AI calls extract_product, then save_product, then done
	callNum := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callNum++
		n := callNum
		mu.Unlock()

		var resp map[string]interface{}
		switch n {
		case 1:
			// AI decides to extract a product
			resp = openAIToolCallResponse([]map[string]interface{}{
				{
					"id":   "call_1",
					"type": "function",
					"function": map[string]interface{}{
						"name":      "extract_product",
						"arguments": `{"url":"https://shop.com/product/1"}`,
					},
				},
			})
		case 2:
			// AI saves the product
			resp = openAIToolCallResponse([]map[string]interface{}{
				{
					"id":   "call_2",
					"type": "function",
					"function": map[string]interface{}{
						"name":      "save_product",
						"arguments": `{"name":"Test Product","price":99.99,"image_url":"https://img.com/test.jpg","source_url":"https://shop.com/product/1"}`,
					},
				},
			})
		case 3:
			// AI calls done
			resp = openAIToolCallResponse([]map[string]interface{}{
				{
					"id":   "call_3",
					"type": "function",
					"function": map[string]interface{}{
						"name":      "done",
						"arguments": `{}`,
					},
				},
			})
		default:
			resp = openAIStopResponse("Done")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	scraper := &mockFirecrawlScraper{
		extractProductFn: func(ctx context.Context, url string) (*crawler.ProductExtractionResult, error) {
			return &crawler.ProductExtractionResult{
				Name:     "Test Product",
				Price:    99.99,
				ImageURL: "https://img.com/test.jpg",
				Found:    true,
			}, nil
		},
	}

	initialLinks := &crawler.LinkDiscoveryResult{
		PageURL:   "https://shop.com",
		PageTitle: "Test Shop",
		Links: []string{
			"https://shop.com/product/1",
			"https://shop.com/product/2",
		},
	}

	orch := crawler.NewOrchestrator("test-api-key", "gpt-5-mini", server.URL, scraper)
	cfg := crawler.CrawlConfig{
		URL:         "https://shop.com",
		Timeout:     30 * time.Second,
		MinProducts: 1,
		MaxScrapes:  50,
	}

	result, err := orch.Run(context.Background(), initialLinks, cfg, nil)
	require.NoError(t, err)

	assert.Len(t, result.Products, 1)
	assert.Equal(t, "Test Product", result.Products[0].Name)
	assert.True(t, result.TotalTokensUsed > 0)
}

func TestOrchestrator_SafetyCapBreaksLoop(t *testing.T) {
	callNum := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callNum++
		mu.Unlock()

		// AI always tries to discover more links — should be stopped by safety cap
		resp := openAIToolCallResponse([]map[string]interface{}{
			{
				"id":   "call_n",
				"type": "function",
				"function": map[string]interface{}{
					"name":      "discover_links",
					"arguments": `{"url":"https://shop.com/page/` + fmt.Sprintf("%d", callNum) + `"}`,
				},
			},
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	scraper := &mockFirecrawlScraper{
		discoverLinksFn: func(ctx context.Context, url string) (*crawler.LinkDiscoveryResult, error) {
			return &crawler.LinkDiscoveryResult{
				PageTitle: "Page",
				Links:     []string{"https://shop.com/page/next"},
			}, nil
		},
	}

	initialLinks := &crawler.LinkDiscoveryResult{
		PageURL:   "https://shop.com",
		PageTitle: "Test Shop",
		Links:     []string{"https://shop.com/page/1"},
	}

	orch := crawler.NewOrchestrator("test-api-key", "gpt-5-mini", server.URL, scraper)
	cfg := crawler.CrawlConfig{
		URL:         "https://shop.com",
		Timeout:     30 * time.Second,
		MinProducts: 20,
		MaxScrapes:  3, // Very low cap to test safety
	}

	result, err := orch.Run(context.Background(), initialLinks, cfg, nil)
	require.NoError(t, err)
	assert.True(t, result.SafetyCapHit)
}

func TestOrchestrator_SkipVisitedURLs(t *testing.T) {
	callNum := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callNum++
		n := callNum
		mu.Unlock()

		var resp map[string]interface{}
		switch n {
		case 1:
			// AI tries to discover links on a URL that's already the initial URL
			resp = openAIToolCallResponse([]map[string]interface{}{
				{
					"id":   "call_1",
					"type": "function",
					"function": map[string]interface{}{
						"name":      "discover_links",
						"arguments": `{"url":"https://shop.com"}`,
					},
				},
			})
		default:
			resp = openAIToolCallResponse([]map[string]interface{}{
				{
					"id":   "call_done",
					"type": "function",
					"function": map[string]interface{}{
						"name":      "done",
						"arguments": `{}`,
					},
				},
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	scrapeCalled := false
	scraper := &mockFirecrawlScraper{
		discoverLinksFn: func(ctx context.Context, url string) (*crawler.LinkDiscoveryResult, error) {
			scrapeCalled = true
			return &crawler.LinkDiscoveryResult{}, nil
		},
	}

	initialLinks := &crawler.LinkDiscoveryResult{
		PageURL:   "https://shop.com",
		PageTitle: "Test Shop",
		Links:     []string{},
	}

	orch := crawler.NewOrchestrator("test-api-key", "gpt-5-mini", server.URL, scraper)
	cfg := crawler.CrawlConfig{
		URL:         "https://shop.com",
		Timeout:     30 * time.Second,
		MinProducts: 1,
		MaxScrapes:  50,
	}

	_, err := orch.Run(context.Background(), initialLinks, cfg, nil)
	require.NoError(t, err)

	// Scraper should NOT have been called since the URL was already visited (initial URL)
	assert.False(t, scrapeCalled, "Firecrawl should not be called for already-visited URLs")
}
```

Note: the `TestOrchestrator_SafetyCapBreaksLoop` test needs `fmt` imported — add it to the imports.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/crawler/ -run "TestOrchestrator" -v` from `backend/`
Expected: FAIL — `NewOrchestrator`, `Orchestrator`, `OrchestratorResult` not defined.

- [ ] **Step 3: Write the implementation**

Create `backend/internal/crawler/orchestrator.go`:

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

// OrchestratorResult contains the outcome of an orchestrator crawl loop.
type OrchestratorResult struct {
	Products       []RawProduct
	ScrapeCount    int
	TotalTokensUsed int
	AIRequestCount int
	SafetyCapHit   bool
	DoneByAI       bool
}

// OrchestratorLogger is an optional callback for logging orchestrator events.
type OrchestratorLogger func(event string, message string)

// Orchestrator manages the AI-driven crawl loop using OpenAI function calling.
type Orchestrator struct {
	client  *openai.Client
	model   string
	scraper FirecrawlScraper
}

// NewOrchestrator creates an Orchestrator with the given OpenAI credentials and Firecrawl scraper.
func NewOrchestrator(apiKey, model, baseURL string, scraper FirecrawlScraper) *Orchestrator {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	client := openai.NewClient(opts...)
	return &Orchestrator{
		client:  &client,
		model:   model,
		scraper: scraper,
	}
}

// Run executes the AI-driven crawl loop.
func (o *Orchestrator) Run(ctx context.Context, initialLinks *LinkDiscoveryResult, cfg CrawlConfig, logger OrchestratorLogger) (*OrchestratorResult, error) {
	log := func(event, msg string) {
		if logger != nil {
			logger(event, msg)
		}
	}

	handlers := NewToolHandlers(cfg.MinProducts)

	// Mark the initial URL as visited
	handlers.MarkVisited(cfg.URL)
	if initialLinks.PageURL != "" && initialLinks.PageURL != cfg.URL {
		handlers.MarkVisited(initialLinks.PageURL)
	}

	// Build initial messages
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(buildSystemPrompt(cfg.MinProducts, cfg.URL)),
		openai.UserMessage(buildInitialMessage(initialLinks)),
	}

	tools := buildToolDefinitions()

	var totalTokens int
	var aiRequestCount int
	var done bool

	log("ORCHESTRATOR_START", fmt.Sprintf("Starting orchestrator loop for %s, target: %d products", cfg.URL, cfg.MinProducts))

	for !done {
		if ctx.Err() != nil {
			log("TOOL_ERROR", fmt.Sprintf("Context cancelled: %v", ctx.Err()))
			break
		}

		if handlers.ScrapeCount() >= cfg.MaxScrapes {
			log("SAFETY_CAP", fmt.Sprintf("Safety cap reached (%d scrape calls). Ending loop with %d products",
				cfg.MaxScrapes, len(handlers.SavedProducts())))
			return &OrchestratorResult{
				Products:        handlers.SavedProducts(),
				ScrapeCount:     handlers.ScrapeCount(),
				TotalTokensUsed: totalTokens,
				AIRequestCount:  aiRequestCount,
				SafetyCapHit:    true,
			}, nil
		}

		// Call OpenAI
		completion, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
			Messages: messages,
			Tools:    tools,
			Model:    o.model,
		})
		if err != nil {
			return nil, fmt.Errorf("OpenAI API error: %w", err)
		}

		aiRequestCount++
		totalTokens += int(completion.Usage.TotalTokens)
		log("AI_TOKENS", fmt.Sprintf("OpenAI call used %d tokens (total: %d)", completion.Usage.TotalTokens, totalTokens))

		if len(completion.Choices) == 0 {
			break
		}

		choice := completion.Choices[0]

		// If the model stopped without tool calls, we're done
		if choice.FinishReason == "stop" {
			break
		}

		// Process tool calls
		if len(choice.Message.ToolCalls) == 0 {
			break
		}

		// Append the assistant message with tool calls
		messages = append(messages, choice.Message.ToParam())

		for _, toolCall := range choice.Message.ToolCalls {
			result := o.handleToolCall(ctx, toolCall, handlers, log)
			messages = append(messages, openai.ToolMessage(result, toolCall.ID))

			if toolCall.Function.Name == "done" {
				done = true
			}
		}
	}

	log("ORCHESTRATOR_DONE", fmt.Sprintf("Orchestrator finished. %d products saved in %d scrape calls",
		len(handlers.SavedProducts()), handlers.ScrapeCount()))

	return &OrchestratorResult{
		Products:        handlers.SavedProducts(),
		ScrapeCount:     handlers.ScrapeCount(),
		TotalTokensUsed: totalTokens,
		AIRequestCount:  aiRequestCount,
		DoneByAI:        done,
	}, nil
}

// handleToolCall dispatches a tool call and returns the result string.
func (o *Orchestrator) handleToolCall(ctx context.Context, tc openai.ChatCompletionMessageToolCall, handlers *ToolHandlers, log OrchestratorLogger) string {
	name := tc.Function.Name
	args := tc.Function.Arguments

	log("TOOL_CALL", fmt.Sprintf("Tool call: %s(%s)", name, args))

	var result string

	switch name {
	case "discover_links":
		result = o.handleDiscoverLinks(ctx, args, handlers, log)
	case "extract_product":
		result = o.handleExtractProduct(ctx, args, handlers, log)
	case "save_product":
		result = o.handleSaveProduct(args, handlers, log)
	case "get_status":
		result = handlers.GetStatus()
	case "done":
		result = "Crawl loop ended."
	default:
		result = fmt.Sprintf("Unknown tool: %s", name)
	}

	log("TOOL_RESULT", fmt.Sprintf("%s result: %s", name, truncateForLog(result, 200)))
	return result
}

// handleDiscoverLinks processes a discover_links tool call.
func (o *Orchestrator) handleDiscoverLinks(ctx context.Context, argsJSON string, handlers *ToolHandlers, log OrchestratorLogger) string {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Invalid arguments: %v", err)
	}

	if handlers.IsVisited(args.URL) {
		log("DUPLICATE_SKIP", fmt.Sprintf("Skipping already-visited URL: %s", args.URL))
		return fmt.Sprintf("Already visited %s. Try a different page.", args.URL)
	}

	handlers.MarkVisited(args.URL)
	handlers.IncrementScrapeCount()

	result, err := o.scraper.DiscoverLinks(ctx, args.URL)
	if err != nil {
		log("TOOL_ERROR", fmt.Sprintf("discover_links failed for %s: %v", args.URL, err))
		return fmt.Sprintf("Failed to fetch %s: %v", args.URL, err)
	}

	log("TOOL_RESULT", fmt.Sprintf("discover_links returned %d links for %s", len(result.Links), args.URL))

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Page: \"%s\"\n", result.PageTitle))
	sb.WriteString(fmt.Sprintf("Links (%d found):\n", len(result.Links)))
	for _, link := range result.Links {
		sb.WriteString(fmt.Sprintf("- %s\n", link))
	}
	return sb.String()
}

// handleExtractProduct processes an extract_product tool call.
func (o *Orchestrator) handleExtractProduct(ctx context.Context, argsJSON string, handlers *ToolHandlers, log OrchestratorLogger) string {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Invalid arguments: %v", err)
	}

	if handlers.IsVisited(args.URL) {
		log("DUPLICATE_SKIP", fmt.Sprintf("Skipping already-visited URL: %s", args.URL))
		return fmt.Sprintf("Already visited %s. Try a different page.", args.URL)
	}

	handlers.MarkVisited(args.URL)
	handlers.IncrementScrapeCount()

	result, err := o.scraper.ExtractProduct(ctx, args.URL)
	if err != nil {
		log("TOOL_ERROR", fmt.Sprintf("extract_product failed for %s: %v", args.URL, err))
		return fmt.Sprintf("Failed to extract product from %s: %v", args.URL, err)
	}

	if !result.Found {
		return fmt.Sprintf("No product data found at %s. This may not be a product page — try discover_links instead.", args.URL)
	}

	return fmt.Sprintf("Product found:\n  Name: %s\n  Price: %.2f\n  Image: %s\n  Source: %s\nUse save_product to save it.",
		result.Name, result.Price, result.ImageURL, args.URL)
}

// handleSaveProduct processes a save_product tool call.
func (o *Orchestrator) handleSaveProduct(argsJSON string, handlers *ToolHandlers, log OrchestratorLogger) string {
	var args struct {
		Name      string  `json:"name"`
		Price     float64 `json:"price"`
		ImageURL  string  `json:"image_url"`
		SourceURL string  `json:"source_url"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Invalid arguments: %v", err)
	}

	product := RawProduct{
		Name:      args.Name,
		Price:     args.Price,
		ImageURL:  args.ImageURL,
		SourceURL: args.SourceURL,
	}

	result := handlers.SaveProduct(product)

	if strings.Contains(result, "Saved") {
		log("PRODUCT_SAVED", fmt.Sprintf("Saved: %s ($%.2f). Progress: %d/%d",
			args.Name, args.Price, len(handlers.SavedProducts()), handlers.minProducts))
	} else {
		log("PRODUCT_REJECTED", result)
	}

	return result
}

// buildSystemPrompt creates the system prompt for the orchestrator.
func buildSystemPrompt(minProducts int, shopURL string) string {
	return fmt.Sprintf(`You are a product discovery agent for an online price guessing game. Your job is to find and save products from an online shop.

Goal: Save at least %d products from %s.

You have these tools:
- discover_links(url): Explore a page to find links. Use on listing pages, category pages, and pagination pages.
- extract_product(url): Extract product data from a single product page. Returns structured data (name, price, image_url). If extraction returns empty, the page is probably not a product page — try discover_links instead.
- save_product(name, price, image_url, source_url): Save a valid product. Returns your current progress.
- get_status(): Check how many products you've saved and see the list.
- done(): Call when you've saved enough products or exhausted available pages.

Strategy:
1. Start by reviewing the initial page links provided below.
2. Identify which links are product pages and which are category/listing/pagination pages.
3. Use discover_links on category and pagination pages to find more products.
4. Use extract_product on product pages, then save_product with the results.
5. Call done() when you reach %d products or run out of pages to explore.

Avoid:
- Re-scraping URLs you've already visited.
- Following links to non-product areas (blog, about, contact, FAQ, etc.).
- Saving duplicate products.`, minProducts, shopURL, minProducts)
}

// buildInitialMessage creates the first user message with initial links.
func buildInitialMessage(links *LinkDiscoveryResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Shop URL: %s\n", links.PageURL))
	sb.WriteString(fmt.Sprintf("Initial page title: \"%s\"\n", links.PageTitle))
	sb.WriteString(fmt.Sprintf("Links found on initial page (%d):\n", len(links.Links)))
	for _, link := range links.Links {
		sb.WriteString(fmt.Sprintf("- %s\n", link))
	}
	sb.WriteString("\nBegin exploring and saving products.")
	return sb.String()
}

// buildToolDefinitions returns the OpenAI tool definitions for the orchestrator.
func buildToolDefinitions() []openai.ChatCompletionToolUnionParam {
	return []openai.ChatCompletionToolUnionParam{
		openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name:        "discover_links",
			Description: openai.String("Explore a page to find links. Use on listing pages, category pages, and pagination pages."),
			Parameters: openai.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{
						"type":        "string",
						"description": "URL of the page to explore",
					},
				},
				"required": []string{"url"},
			},
		}),
		openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name:        "extract_product",
			Description: openai.String("Extract product data from a single product page. Returns name, price, and image_url."),
			Parameters: openai.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{
						"type":        "string",
						"description": "URL of a product page",
					},
				},
				"required": []string{"url"},
			},
		}),
		openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name:        "save_product",
			Description: openai.String("Save a product to the database. Returns progress info."),
			Parameters: openai.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"name":       map[string]any{"type": "string", "description": "Product name"},
					"price":      map[string]any{"type": "number", "description": "Product price"},
					"image_url":  map[string]any{"type": "string", "description": "Product image URL"},
					"source_url": map[string]any{"type": "string", "description": "Source page URL"},
				},
				"required": []string{"name", "price", "source_url"},
			},
		}),
		openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name:        "get_status",
			Description: openai.String("Check how many products have been saved and see the list."),
			Parameters: openai.FunctionParameters{
				"type":       "object",
				"properties": map[string]any{},
			},
		}),
		openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name:        "done",
			Description: openai.String("Signal that you are finished collecting products."),
			Parameters: openai.FunctionParameters{
				"type":       "object",
				"properties": map[string]any{},
			},
		}),
	}
}

// truncateForLog truncates a string for logging purposes.
func truncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
```

**Note:** The `handleSaveProduct` method references `handlers.minProducts` which is unexported. Fix by making `ToolHandlers.minProducts` accessible via a `MinProducts()` method, or change the log line to use `len(handlers.SavedProducts())` with the target from the method. The simplest fix: add a `MinProducts() int` method to `ToolHandlers`:

Add to `tool_handlers.go`:
```go
// MinProducts returns the target product count.
func (h *ToolHandlers) MinProducts() int {
	return h.minProducts
}
```

And update the `handleSaveProduct` log line to use `handlers.MinProducts()`:
```go
log("PRODUCT_SAVED", fmt.Sprintf("Saved: %s ($%.2f). Progress: %d/%d",
    args.Name, args.Price, len(handlers.SavedProducts()), handlers.MinProducts()))
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/crawler/ -run "TestOrchestrator" -v` from `backend/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/crawler/orchestrator.go backend/internal/crawler/orchestrator_test.go backend/internal/crawler/tool_handlers.go
git commit -m "feat: add Orchestrator with OpenAI function-calling loop"
```

---

### Task 5: Rewrite Crawler to Use Orchestrator

**Files:**
- Rewrite: `backend/internal/crawler/crawler.go`
- Rewrite: `backend/internal/crawler/crawler_test.go`
- Delete: `backend/internal/crawler/ai_client.go`
- Delete: `backend/internal/crawler/ai_client_test.go`
- Delete: `backend/internal/crawler/extractor.go`
- Delete: `backend/internal/crawler/extractor_test.go`

- [ ] **Step 1: Delete old files**

```bash
rm backend/internal/crawler/ai_client.go backend/internal/crawler/ai_client_test.go
rm backend/internal/crawler/extractor.go backend/internal/crawler/extractor_test.go
```

- [ ] **Step 2: Rewrite crawler.go**

Replace the entire content of `backend/internal/crawler/crawler.go`:

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
	// MaxScrapes is the maximum number of Firecrawl scrape calls (safety cap).
	MaxScrapes int
	// Verbose enables extra stdout output (for CLI mode).
	Verbose bool
}

// DefaultCrawlConfig returns config with sensible defaults.
func DefaultCrawlConfig(url string) CrawlConfig {
	return CrawlConfig{
		URL:         url,
		Timeout:     300 * time.Second,
		MinProducts: 20,
		MaxScrapes:  50,
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
	ScrapeCount     int
	AIRequestsCount int
	TotalTokensUsed int
	Duration        time.Duration
	LogFilePath     string
}

// Crawler orchestrates the product crawling pipeline.
type Crawler struct {
	scraper      FirecrawlScraper
	orchestrator *Orchestrator
	shopStore    *store.ShopStore
	crawlStore   *store.CrawlStore
	productStore *store.ProductStore
	logDir       string
}

// New creates a new Crawler with all dependencies.
func New(
	scraper FirecrawlScraper,
	orchestrator *Orchestrator,
	shopStore *store.ShopStore,
	crawlStore *store.CrawlStore,
	productStore *store.ProductStore,
	logDir string,
) *Crawler {
	return &Crawler{
		scraper:      scraper,
		orchestrator: orchestrator,
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

	// 3. Scrape initial page for links
	logger.Log("ORCHESTRATOR_START", fmt.Sprintf("Scraping initial page: %s", cfg.URL))
	report("start", fmt.Sprintf("Scraping initial page: %s", cfg.URL))

	initialLinks, err := c.scraper.DiscoverLinks(ctx, cfg.URL)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to scrape initial page: %v", err)
		logger.Log("ERROR", errMsg)
		if finErr := c.crawlStore.Finish(ctx, crawl.ID, models.CrawlStatusFailed, 0, 0, 0, &errMsg); finErr != nil {
			logger.Log("ERROR", fmt.Sprintf("Failed to finish crawl record: %v", finErr))
		}
		return &CrawlResult{
			CrawlID:     crawl.ID,
			ShopID:      shop.ID,
			Duration:    time.Since(start),
			LogFilePath: logger.FilePath(),
		}, nil
	}

	if len(initialLinks.Links) == 0 {
		errMsg := "No links found on initial page"
		logger.Log("ERROR", errMsg)
		if finErr := c.crawlStore.Finish(ctx, crawl.ID, models.CrawlStatusFailed, 0, 1, 0, &errMsg); finErr != nil {
			logger.Log("ERROR", fmt.Sprintf("Failed to finish crawl record: %v", finErr))
		}
		return &CrawlResult{
			CrawlID:     crawl.ID,
			ShopID:      shop.ID,
			ScrapeCount: 1,
			Duration:    time.Since(start),
			LogFilePath: logger.FilePath(),
		}, nil
	}

	logger.Log("TOOL_RESULT", fmt.Sprintf("Initial page has %d links", len(initialLinks.Links)))
	report("links_found", fmt.Sprintf("Found %d links on initial page", len(initialLinks.Links)))

	// 4. Run orchestrator loop
	orchLogger := func(event, msg string) {
		logger.Log(event, msg)
		report(event, msg)
	}

	orchResult, err := c.orchestrator.Run(ctx, initialLinks, cfg, orchLogger)
	if err != nil {
		errMsg := fmt.Sprintf("Orchestrator error: %v", err)
		logger.Log("ERROR", errMsg)
		if finErr := c.crawlStore.Finish(ctx, crawl.ID, models.CrawlStatusFailed, 0, orchResult.ScrapeCount+1, orchResult.AIRequestCount, &errMsg); finErr != nil {
			logger.Log("ERROR", fmt.Sprintf("Failed to finish crawl record: %v", finErr))
		}
		return &CrawlResult{
			CrawlID:         crawl.ID,
			ShopID:          shop.ID,
			ScrapeCount:     orchResult.ScrapeCount + 1,
			AIRequestsCount: orchResult.AIRequestCount,
			TotalTokensUsed: orchResult.TotalTokensUsed,
			Duration:        time.Since(start),
			LogFilePath:     logger.FilePath(),
		}, nil
	}

	// 5. Save products to DB
	savedCount := 0
	for _, p := range orchResult.Products {
		_, err := c.productStore.Create(ctx, shop.ID, crawl.ID, p.Name, p.Price, p.ImageURL, p.SourceURL)
		if err != nil {
			logger.Log("ERROR", fmt.Sprintf("Failed to save product %s: %v", p.Name, err))
			continue
		}
		savedCount++
	}

	// 6. Update shop last_crawled_at
	if err := c.shopStore.UpdateLastCrawled(ctx, shop.ID); err != nil {
		logger.Log("ERROR", fmt.Sprintf("Failed to update shop last_crawled_at: %v", err))
	}

	// 7. Finish crawl record
	duration := time.Since(start)
	status := models.CrawlStatusCompleted
	var errMsg *string
	if savedCount < cfg.MinProducts {
		msg := fmt.Sprintf("found only %d products (minimum: %d)", savedCount, cfg.MinProducts)
		errMsg = &msg
		logger.Log("ERROR", msg)
	}

	totalScrapes := orchResult.ScrapeCount + 1 // +1 for initial page scrape
	if err := c.crawlStore.Finish(ctx, crawl.ID, status, savedCount, totalScrapes, orchResult.AIRequestCount, errMsg); err != nil {
		logger.Log("ERROR", fmt.Sprintf("Failed to finish crawl record: %v", err))
	}

	logger.Log("ORCHESTRATOR_DONE", fmt.Sprintf("Crawl complete: %d products, %d scrapes, %d AI requests, %d tokens, %v",
		savedCount, totalScrapes, orchResult.AIRequestCount, orchResult.TotalTokensUsed, duration))
	report("done", fmt.Sprintf("Crawl complete: %d products, %d scrapes", savedCount, totalScrapes))

	return &CrawlResult{
		CrawlID:         crawl.ID,
		ShopID:          shop.ID,
		ProductsFound:   savedCount,
		ScrapeCount:     totalScrapes,
		AIRequestsCount: orchResult.AIRequestCount,
		TotalTokensUsed: orchResult.TotalTokensUsed,
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

- [ ] **Step 3: Rewrite crawler_test.go**

Replace the entire content of `backend/internal/crawler/crawler_test.go`:

```go
package crawler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/jzy/howmuchyousay/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestCrawler_RunWithOrchestrator(t *testing.T) {
	pool := setupTestDB(t)
	logDir := t.TempDir()

	// Mock Firecrawl scraper
	scraper := &mockFirecrawlScraper{
		discoverLinksFn: func(ctx context.Context, url string) (*crawler.LinkDiscoveryResult, error) {
			return &crawler.LinkDiscoveryResult{
				PageURL:   url,
				PageTitle: "Test Shop",
				Links: []string{
					"https://shop.com/product/1",
					"https://shop.com/product/2",
				},
			}, nil
		},
		extractProductFn: func(ctx context.Context, url string) (*crawler.ProductExtractionResult, error) {
			if url == "https://shop.com/product/1" {
				return &crawler.ProductExtractionResult{
					Name: "Product One", Price: 99.99, ImageURL: "https://img.com/1.jpg", Found: true,
				}, nil
			}
			return &crawler.ProductExtractionResult{
				Name: "Product Two", Price: 199.99, ImageURL: "https://img.com/2.jpg", Found: true,
			}, nil
		},
	}

	// Mock OpenAI: extract product 1, save, extract product 2, save, done
	callNum := 0
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callNum++
		n := callNum
		mu.Unlock()

		var resp map[string]interface{}
		switch n {
		case 1:
			resp = openAIToolCallResponse([]map[string]interface{}{
				{"id": "c1", "type": "function", "function": map[string]interface{}{
					"name": "extract_product", "arguments": `{"url":"https://shop.com/product/1"}`,
				}},
			})
		case 2:
			resp = openAIToolCallResponse([]map[string]interface{}{
				{"id": "c2", "type": "function", "function": map[string]interface{}{
					"name": "save_product", "arguments": `{"name":"Product One","price":99.99,"image_url":"https://img.com/1.jpg","source_url":"https://shop.com/product/1"}`,
				}},
			})
		case 3:
			resp = openAIToolCallResponse([]map[string]interface{}{
				{"id": "c3", "type": "function", "function": map[string]interface{}{
					"name": "extract_product", "arguments": `{"url":"https://shop.com/product/2"}`,
				}},
			})
		case 4:
			resp = openAIToolCallResponse([]map[string]interface{}{
				{"id": "c4", "type": "function", "function": map[string]interface{}{
					"name": "save_product", "arguments": `{"name":"Product Two","price":199.99,"image_url":"https://img.com/2.jpg","source_url":"https://shop.com/product/2"}`,
				}},
			})
		case 5:
			resp = openAIToolCallResponse([]map[string]interface{}{
				{"id": "c5", "type": "function", "function": map[string]interface{}{
					"name": "done", "arguments": `{}`,
				}},
			})
		default:
			resp = openAIStopResponse("Done")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	orch := crawler.NewOrchestrator("test-key", "gpt-5-mini", server.URL, scraper)

	c := crawler.New(
		scraper,
		orch,
		store.NewShopStore(pool),
		store.NewCrawlStore(pool),
		store.NewProductStore(pool),
		logDir,
	)

	cfg := crawler.CrawlConfig{
		URL:         "https://shop.com",
		Timeout:     30 * time.Second,
		MinProducts: 1,
		MaxScrapes:  50,
	}

	result, err := c.Run(context.Background(), cfg, nil)
	require.NoError(t, err)

	assert.Equal(t, 2, result.ProductsFound)
	assert.NotEmpty(t, result.CrawlID)
	assert.NotEmpty(t, result.ShopID)
	assert.True(t, result.TotalTokensUsed > 0)
}

func TestCrawler_RunInitialScrapeError(t *testing.T) {
	pool := setupTestDB(t)
	logDir := t.TempDir()

	scraper := &mockFirecrawlScraper{
		discoverLinksFn: func(ctx context.Context, url string) (*crawler.LinkDiscoveryResult, error) {
			return nil, fmt.Errorf("firecrawl API error: 401 unauthorized")
		},
	}

	// Orchestrator won't be called if initial scrape fails
	orch := crawler.NewOrchestrator("test-key", "gpt-5-mini", "", scraper)

	c := crawler.New(
		scraper,
		orch,
		store.NewShopStore(pool),
		store.NewCrawlStore(pool),
		store.NewProductStore(pool),
		logDir,
	)

	cfg := crawler.CrawlConfig{
		URL:         "https://shop.com",
		Timeout:     10 * time.Second,
		MinProducts: 1,
		MaxScrapes:  50,
	}

	result, err := c.Run(context.Background(), cfg, nil)
	require.NoError(t, err) // Crawl itself doesn't error — it marks the crawl as failed
	assert.Equal(t, 0, result.ProductsFound)
}

func TestCrawler_RunNoLinksOnInitialPage(t *testing.T) {
	pool := setupTestDB(t)
	logDir := t.TempDir()

	scraper := &mockFirecrawlScraper{
		discoverLinksFn: func(ctx context.Context, url string) (*crawler.LinkDiscoveryResult, error) {
			return &crawler.LinkDiscoveryResult{
				PageURL:   url,
				PageTitle: "Empty Shop",
				Links:     []string{},
			}, nil
		},
	}

	orch := crawler.NewOrchestrator("test-key", "gpt-5-mini", "", scraper)

	c := crawler.New(
		scraper,
		orch,
		store.NewShopStore(pool),
		store.NewCrawlStore(pool),
		store.NewProductStore(pool),
		logDir,
	)

	cfg := crawler.CrawlConfig{
		URL:         "https://shop.com",
		Timeout:     10 * time.Second,
		MinProducts: 1,
		MaxScrapes:  50,
	}

	result, err := c.Run(context.Background(), cfg, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, result.ProductsFound)
}

// Helper functions shared with orchestrator_test.go
// (These are duplicated here since they're in the _test package and both files need them.)

func openAIToolCallResponse(toolCalls []map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"id":     "chatcmpl-test",
		"object": "chat.completion",
		"model":  "gpt-5-mini",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":       "assistant",
					"content":    nil,
					"tool_calls": toolCalls,
				},
				"finish_reason": "tool_calls",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     100,
			"completion_tokens": 50,
			"total_tokens":      150,
		},
	}
}

func openAIStopResponse(content string) map[string]interface{} {
	return map[string]interface{}{
		"id":     "chatcmpl-test",
		"object": "chat.completion",
		"model":  "gpt-5-mini",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     50,
			"completion_tokens": 20,
			"total_tokens":      70,
		},
	}
}
```

**Note:** The `mockFirecrawlScraper` type is defined in `orchestrator_test.go` and `crawler_test.go` — both are in `package crawler_test`. Move the mock and helpers to a shared `helpers_test.go` file instead:

Create `backend/internal/crawler/helpers_test.go`:
```go
package crawler_test

import (
	"context"

	"github.com/jzy/howmuchyousay/internal/crawler"
)

// mockFirecrawlScraper implements FirecrawlScraper for testing.
type mockFirecrawlScraper struct {
	discoverLinksFn  func(ctx context.Context, url string) (*crawler.LinkDiscoveryResult, error)
	extractProductFn func(ctx context.Context, url string) (*crawler.ProductExtractionResult, error)
}

func (m *mockFirecrawlScraper) DiscoverLinks(ctx context.Context, url string) (*crawler.LinkDiscoveryResult, error) {
	if m.discoverLinksFn != nil {
		return m.discoverLinksFn(ctx, url)
	}
	return &crawler.LinkDiscoveryResult{}, nil
}

func (m *mockFirecrawlScraper) ExtractProduct(ctx context.Context, url string) (*crawler.ProductExtractionResult, error) {
	if m.extractProductFn != nil {
		return m.extractProductFn(ctx, url)
	}
	return &crawler.ProductExtractionResult{Found: false}, nil
}

// openAIToolCallResponse creates a mock OpenAI response with tool calls.
func openAIToolCallResponse(toolCalls []map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"id":     "chatcmpl-test",
		"object": "chat.completion",
		"model":  "gpt-5-mini",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":       "assistant",
					"content":    nil,
					"tool_calls": toolCalls,
				},
				"finish_reason": "tool_calls",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     100,
			"completion_tokens": 50,
			"total_tokens":      150,
		},
	}
}

// openAIStopResponse creates a mock OpenAI response that ends the conversation.
func openAIStopResponse(content string) map[string]interface{} {
	return map[string]interface{}{
		"id":     "chatcmpl-test",
		"object": "chat.completion",
		"model":  "gpt-5-mini",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     50,
			"completion_tokens": 20,
			"total_tokens":      70,
		},
	}
}
```

Then remove the duplicate definitions from both `orchestrator_test.go` and `crawler_test.go`.

- [ ] **Step 4: Verify build compiles**

Run: `go build ./...` from `backend/`
Expected: Will fail until `cmd/crawler/main.go` is updated (Task 6). Run tests for the crawler package only:

Run: `go test ./internal/crawler/ -run "TestCrawler_|TestOrchestrator_|TestToolHandlers|TestFirecrawlScraper|TestConvert|TestNewFirecrawlClient" -v` from `backend/`
Expected: All tests PASS (DB-dependent tests need the test database running).

- [ ] **Step 5: Commit**

```bash
git rm backend/internal/crawler/ai_client.go backend/internal/crawler/ai_client_test.go
git rm backend/internal/crawler/extractor.go backend/internal/crawler/extractor_test.go
git add backend/internal/crawler/crawler.go backend/internal/crawler/crawler_test.go backend/internal/crawler/helpers_test.go
git commit -m "feat: rewrite Crawler to use AI-driven orchestrator loop"
```

---

### Task 6: Update CLI Entry Point

**Files:**
- Modify: `backend/cmd/crawler/main.go`

- [ ] **Step 1: Rewrite main.go**

Replace the entire content of `backend/cmd/crawler/main.go`:

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
	timeoutFlag := flag.Duration("timeout", 0, "Maximum crawl duration (0 uses CRAWL_TIMEOUT env or 300s default)")
	minProductsFlag := flag.Int("min-products", 20, "Minimum number of products to collect")
	maxScrapesFlag := flag.Int("max-scrapes", 0, "Max Firecrawl scrape calls (0 uses FIRECRAWL_MAX_SCRAPES env or 50 default)")
	verboseFlag := flag.Bool("verbose", false, "Enable verbose output")

	flag.Parse()

	if *urlFlag == "" {
		fmt.Fprintln(os.Stderr, "Error: --url flag is required")
		fmt.Fprintln(os.Stderr, "Usage: crawler --url <shop_url> [--timeout 300s] [--min-products 20] [--max-scrapes 50] [--verbose]")
		os.Exit(1)
	}

	cfg := config.Load()

	if cfg.FirecrawlAPIKey == "" {
		fmt.Fprintln(os.Stderr, "Error: FIRECRAWL_API_KEY must be set")
		os.Exit(1)
	}

	if cfg.OpenAIAPIKey == "" {
		fmt.Fprintln(os.Stderr, "Error: OPENAI_API_KEY must be set (required for orchestrator)")
		os.Exit(1)
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

	// Create Firecrawl scraper
	scraper, err := crawler.NewFirecrawlClient(cfg.FirecrawlAPIKey, cfg.FirecrawlAPIURL)
	if err != nil {
		log.Fatalf("Failed to create Firecrawl client: %v", err)
	}

	// Create orchestrator
	orch := crawler.NewOrchestrator(cfg.OpenAIAPIKey, cfg.OpenAIModel, "", scraper)

	c := crawler.New(
		scraper,
		orch,
		store.NewShopStore(pool),
		store.NewCrawlStore(pool),
		store.NewProductStore(pool),
		cfg.LogDir,
	)

	// Resolve timeout
	timeout := *timeoutFlag
	if timeout == 0 {
		timeout = time.Duration(cfg.CrawlTimeout) * time.Second
	}

	// Resolve max scrapes
	maxScrapes := *maxScrapesFlag
	if maxScrapes == 0 {
		maxScrapes = cfg.FirecrawlMaxScrapes
	}

	crawlCfg := crawler.CrawlConfig{
		URL:         *urlFlag,
		Timeout:     timeout,
		MinProducts: *minProductsFlag,
		MaxScrapes:  maxScrapes,
		Verbose:     *verboseFlag,
	}

	if *verboseFlag {
		fmt.Printf("Starting crawl of %s\n", *urlFlag)
		fmt.Printf("  Timeout: %s\n", timeout)
		fmt.Printf("  Min products: %d\n", *minProductsFlag)
		fmt.Printf("  Max scrapes: %d\n", maxScrapes)
		fmt.Printf("  Firecrawl API: %s\n", cfg.FirecrawlAPIURL)
		fmt.Printf("  OpenAI model: %s\n", cfg.OpenAIModel)
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
	fmt.Printf("  Scrapes:      %d\n", result.ScrapeCount)
	fmt.Printf("  AI requests:  %d\n", result.AIRequestsCount)
	fmt.Printf("  Tokens used:  %d\n", result.TotalTokensUsed)
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

- [ ] **Step 2: Verify full build compiles**

Run: `go build ./...` from `backend/`
Expected: PASS — all compilation errors should be resolved.

- [ ] **Step 3: Run all crawler tests**

Run: `go test ./internal/crawler/ -v` from `backend/`
Expected: All tests PASS.

- [ ] **Step 4: Commit**

```bash
git add backend/cmd/crawler/main.go
git commit -m "feat: update CLI to use orchestrator-based crawler"
```

---

### Task 7: Final Verification and Cleanup

- [ ] **Step 1: Run full build**

Run: `go build ./...` from `backend/`
Expected: PASS

- [ ] **Step 2: Run all crawler tests**

Run: `go test ./internal/crawler/ -v -count=1` from `backend/`
Expected: All tests PASS.

- [ ] **Step 3: Run store tests separately**

Run: `go test ./internal/store/ -v -count=1` from `backend/`
Expected: All tests PASS.

- [ ] **Step 4: Verify no orphan references to old types**

Search the codebase for references to removed types:
- `FirecrawlCrawler` (old interface)
- `AIClient` (old interface)
- `ExtractProducts` (old method)
- `CrawlSite` (old method)
- `PageResult` (old struct)
- `MaxDepth` (old config)
- `FirecrawlMaxDepth` (old config)

None should appear in non-test Go files.

- [ ] **Step 5: Verify .env.example matches config.go**

Check that every field in `Config` struct has a corresponding env var in `.env.example` and vice versa.

- [ ] **Step 6: Commit any cleanup**

If any cleanup was needed:
```bash
git add -A
git commit -m "chore: cleanup orphan references from old crawl pipeline"
```
