# AI-Driven Crawl Loop Design

## Overview

Replace the current Firecrawl-crawl-then-extract pipeline with an AI-driven orchestrator loop. An OpenAI model acts as a navigation brain that uses Firecrawl as tools to explore shop pages, extract products, and decide when it has collected enough.

The system has three layers:
1. **Firecrawl** — content acquisition and structured extraction via `ScrapeURL`
2. **OpenAI orchestrator** — navigation decisions via function calling
3. **Go crawl loop** — executes tool calls, persists data, enforces safety caps

## Architecture

### Data Flow

```
User provides shop URL
  → Go code scrapes initial URL (link discovery mode)
  → Initial links + page title fed to orchestrator as first message
  → Orchestrator tool-use loop:
      → AI calls discover_links(url) → Go calls Firecrawl → returns links
      → AI calls extract_product(url) → Go calls Firecrawl with JSON schema → returns product data
      → AI calls save_product(...) → Go validates + saves to DB → returns progress
      → AI calls get_status() → Go returns current state
      → AI calls done() → loop ends
  → Go finalizes crawl record
```

### Firecrawl Scrape Modes

Two distinct `ScrapeURL` configurations:

**Link discovery mode** — used on listing, category, and pagination pages:
- `Formats: ["links"]`
- `OnlyMainContent: true`
- Returns: list of URLs from the page + page title from metadata

**Product extraction mode** — used on individual product pages:
- `Formats: ["json"]`
- `JsonOptions.Schema`: `{name: string, price: number, image_url: string}`
- `JsonOptions.Prompt`: "Extract the product name, price, and main product image URL from this product page."
- Returns: structured JSON with product data, extracted by Firecrawl's server-side LLM

No `CrawlURL` is used anywhere. All Firecrawl interaction is single-page `ScrapeURL` calls.

## Tool Definitions

### discover_links(url)

Explores any page to find links. Used on listing pages, category pages, and pagination pages.

OpenAI function schema:
```json
{
  "name": "discover_links",
  "parameters": {
    "type": "object",
    "properties": {
      "url": { "type": "string", "description": "URL of the page to explore" }
    },
    "required": ["url"]
  }
}
```

Firecrawl call:
```go
ScrapeURL(url, &ScrapeParams{
    Formats:         []string{"links"},
    OnlyMainContent: &true,
})
```

Returns to orchestrator: page title + list of link URLs. The Firecrawl `links` format returns bare URLs without anchor text — the AI classifies links based on URL patterns alone. Example:
```
Page: "Electronics - Best Buy"
Links:
- https://shop.com/product/iphone-15
- https://shop.com/product/samsung-galaxy
- https://shop.com/category/laptops
- https://shop.com/electronics?page=2
```

### extract_product(url)

Extracts structured product data from a single product page using Firecrawl's LLM.

OpenAI function schema:
```json
{
  "name": "extract_product",
  "parameters": {
    "type": "object",
    "properties": {
      "url": { "type": "string", "description": "URL of a product page" }
    },
    "required": ["url"]
  }
}
```

Firecrawl call:
```go
ScrapeURL(url, &ScrapeParams{
    Formats: []string{"json"},
    JsonOptions: &JsonOptions{
        Schema: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "name":      map[string]any{"type": "string"},
                "price":     map[string]any{"type": "number"},
                "image_url": map[string]any{"type": "string"},
            },
            "required": []string{"name", "price"},
        },
        Prompt: &extractPrompt,
    },
})
```

Returns to orchestrator: structured JSON, or error message if extraction found nothing.

### save_product(name, price, image_url, source_url)

Validates and persists a product to the database.

OpenAI function schema:
```json
{
  "name": "save_product",
  "parameters": {
    "type": "object",
    "properties": {
      "name": { "type": "string" },
      "price": { "type": "number" },
      "image_url": { "type": "string" },
      "source_url": { "type": "string" }
    },
    "required": ["name", "price", "source_url"]
  }
}
```

Go code runs `ValidateProducts`, saves to DB, returns progress:
```
Saved "iPhone 15 Pro Max" ($999.00). Progress: 8/20 products.
Saved so far: iPhone 15 Pro Max, Samsung Galaxy S24, MacBook Air M3, ...
```

### get_status()

Returns current crawl progress. No parameters.

Returns: product count, target, list of saved product names + source URLs.

### done()

Signals the orchestrator is finished. No parameters. Go code breaks the loop.

## Orchestrator System Prompt

```
You are a product discovery agent for an online price guessing game. Your job is to find and save products from an online shop.

Goal: Save at least {min_products} products from {shop_url}.

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
5. Call done() when you reach {min_products} products or run out of pages to explore.

Avoid:
- Re-scraping URLs you've already visited.
- Following links to non-product areas (blog, about, contact, FAQ, etc.).
- Saving duplicate products.
```

## Go Loop

### Pseudocode

```
1. Scrape initial URL via discover_links (Firecrawl ScrapeURL with links format)
2. Build messages: [system_prompt, user_message_with_initial_links]
3. Initialize: visitedURLs = {initial_url}, scrapeCount = 1, done = false
4. Loop:
   a. Call OpenAI chat completion with messages + tool definitions
   b. Track tokens used
   c. If finish_reason == "stop" → break
   d. If finish_reason == "tool_calls":
      - Append assistant message to conversation
      - For each tool call:
        - discover_links(url):
            if url in visitedURLs → return "Already visited"
            else → call Firecrawl ScrapeURL (links), add to visitedURLs, scrapeCount++
        - extract_product(url):
            if url in visitedURLs → return "Already visited"
            else → call Firecrawl ScrapeURL (json), add to visitedURLs, scrapeCount++
        - save_product(data):
            validate → save to DB → return progress string
        - get_status():
            return progress string
        - done():
            set done = true
      - Append all tool results to conversation
   e. If done → break
   f. If scrapeCount >= maxScrapes → break with SAFETY_CAP log
   g. If ctx.Err() != nil → break with timeout log
5. Finalize crawl record with saved product count, scrape count, token usage
```

### Initial Message

Before the loop starts, Go code scrapes the initial URL and constructs the first user message:

```
Shop URL: {url}
Initial page title: "{title}"
Links found on initial page:
- {link1_url}
- {link2_url}
...

Begin exploring and saving products.
```

## Error Handling

### Firecrawl Failures
- `discover_links` fails: return `"Failed to fetch {url}: {error}"` to orchestrator. AI skips the URL.
- `extract_product` fails or returns empty: return `"No product data found at {url}. This may not be a product page."` AI can try `discover_links` on it instead.
- Rate limit from Firecrawl: Go code retries once after a wait, then reports failure.

### Orchestrator AI Failures
- OpenAI API error mid-loop: crawl ends with products saved so far. Status `"failed"`.
- Invalid tool call arguments: return error as tool result, AI retries.
- AI loops on same URLs: visited-URL tracking returns "Already visited" without Firecrawl calls. Safety cap on scrape count prevents infinite loops.

### Duplicate Handling
- **URL dedup:** `visitedURLs` map tracks all URLs passed to `discover_links` and `extract_product`. Repeated URLs get instant response, no Firecrawl call.
- **Product dedup:** `save_product` checks product name + source_url against already-saved products in current crawl. Duplicates are rejected with message.

### Validation Failures
- `save_product` runs existing `ValidateProducts` logic. Rejected products get: `"Product rejected: {reason}. Not counted. Progress: 8/20."`.

### Timeout
- Context timeout covers the entire crawl (all Firecrawl + OpenAI calls). Default 300s (5 minutes).
- When context expires, Go code catches it, finalizes with products saved so far.

### Empty Results
- Initial `discover_links` returns zero links: crawl ends immediately, status `"failed"`, error `"No links found on initial page"`.
- AI calls `done()` with zero products: status `"completed"` with warning `"found only 0 products (minimum: 20)"`.

## Logging & Observability

### Log Events

| Event | When | Example |
|-------|------|---------|
| `ORCHESTRATOR_START` | Loop begins | `"Starting orchestrator loop for https://shop.com, target: 20 products"` |
| `TOOL_CALL` | AI calls any tool | `"Tool call: discover_links(https://shop.com/category/phones)"` |
| `TOOL_RESULT` | Tool returns to AI | `"discover_links returned 15 links for https://shop.com/category/phones"` |
| `TOOL_ERROR` | Tool call fails | `"extract_product failed for https://shop.com/about: no product data"` |
| `DUPLICATE_SKIP` | URL already visited | `"Skipping already-visited URL: https://shop.com/page/2"` |
| `PRODUCT_SAVED` | Product persisted | `"Saved: iPhone 15 Pro Max ($999.00). Progress: 8/20"` |
| `PRODUCT_REJECTED` | Validation failed | `"Product rejected: price $0.00 out of range"` |
| `ORCHESTRATOR_DONE` | AI calls done() | `"Orchestrator finished. 22 products saved in 12 tool calls"` |
| `SAFETY_CAP` | Max scrapes hit | `"Safety cap reached (50 scrape calls). Ending loop with 15 products"` |
| `AI_TOKENS` | After each OpenAI call | `"OpenAI call used 1,250 tokens (total this crawl: 8,400)"` |

### Token Tracking

Each OpenAI completion returns `Usage.TotalTokens`. The Go code accumulates this across the loop. `CrawlResult` gets a new `TotalTokensUsed int` field.

### Progress Reporting

The existing `ProgressFunc` callback fires on key events (tool calls, products saved, loop end) for CLI real-time output.

## Configuration

### New Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `FIRECRAWL_MAX_SCRAPES` | `50` | Max Firecrawl scrape calls per crawl (safety cap) |
| `CRAWL_TIMEOUT` | `300` | Crawl timeout in seconds |

### Removed Environment Variables

| Variable | Reason |
|----------|--------|
| `FIRECRAWL_MAX_DEPTH` | No longer relevant — only `ScrapeURL` is used, not `CrawlURL` |

### Config Struct

```go
type Config struct {
    // ... existing fields ...
    FirecrawlAPIKey     string // stays
    FirecrawlAPIURL     string // stays
    FirecrawlMaxScrapes int    // new, replaces FirecrawlMaxDepth
    CrawlTimeout        int    // new, seconds, default 300
}
```

### CrawlConfig Struct

```go
type CrawlConfig struct {
    URL         string
    Timeout     time.Duration  // default 300s (was 90s)
    MinProducts int            // default 20
    MaxScrapes  int            // new, default 50, replaces MaxDepth
    Verbose     bool
}
```

## Code Changes Summary

### Rewritten
- `crawler.go` — `Run`/`RunWithProgress` replaced by orchestrator loop. Shop/crawl record management, logging, finalization unchanged.
- `ai_client.go` — `AIClient` interface replaced by `Orchestrator` interface managing the tool-use conversation.
- `firecrawl_client.go` — `CrawlSite` replaced by `DiscoverLinks` and `ExtractProduct`. `CrawlParams`/`CrawlURL` removed entirely.

### Removed
- `extractor.go` — `ExtractProductsFromPage`, `HasProductSignals`, `PageHasContent` no longer needed. Product extraction done by Firecrawl's LLM.

### Unchanged
- `validator.go` — still validates products before saving.
- `logger.go` — same structure, new log events added.

### Updated
- `config/config.go` — add `FirecrawlMaxScrapes`, `CrawlTimeout`; remove `FirecrawlMaxDepth`.
- `cmd/crawler/main.go` — wire up new types.
- `.env.example` — reflect new/removed env vars.
- All `*_test.go` files — rewritten to match new interfaces.

### New Interfaces

```go
// Replaces FirecrawlCrawler
type FirecrawlScraper interface {
    DiscoverLinks(ctx context.Context, url string) (*LinkDiscoveryResult, error)
    ExtractProduct(ctx context.Context, url string) (*ProductExtractionResult, error)
}

// Replaces AIClient
type Orchestrator interface {
    RunCrawlLoop(ctx context.Context, initialLinks *LinkDiscoveryResult, cfg CrawlConfig, handlers ToolHandlers) (*OrchestratorResult, error)
}
```
