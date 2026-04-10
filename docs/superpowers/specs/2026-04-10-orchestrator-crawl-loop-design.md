# AI-Driven Orchestrator Crawl Loop Design

**Date:** 2026-04-10
**Status:** Current

## Overview

An AI-driven orchestrator loop where an OpenAI model acts as a navigation brain that uses Firecrawl as tools to explore shop pages, extract products, and decide when it has collected enough. I/O-bound tool calls execute in parallel for performance.

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
      → AI calls extract_and_save_product(url) → Go calls Firecrawl + validates + saves → returns progress
      → AI calls get_status() → Go returns current state
      → AI calls done() → loop ends
  → Go finalizes crawl record
```

### Firecrawl Scrape Modes

Two distinct `ScrapeURL` configurations. No `CrawlURL` is used anywhere — all Firecrawl interaction is single-page `ScrapeURL` calls.

**Link discovery mode** — used on listing, category, and pagination pages:
- `Formats: ["links"]`
- `OnlyMainContent: true`
- Returns: list of URLs from the page + page title from metadata

**Product extraction mode** — used on individual product pages:
- `Formats: ["json"]`
- `JsonOptions.Schema`: `{name: string, price: number, image_url: string}`
- `JsonOptions.Prompt`: "Extract the product name, price (as a number), and main product image URL from this product page."
- Returns: structured JSON with product data, extracted by Firecrawl's server-side LLM

## Interfaces

```go
// FirecrawlScraper defines the interface for single-page scraping via Firecrawl.
type FirecrawlScraper interface {
    DiscoverLinks(ctx context.Context, url string) (*LinkDiscoveryResult, error)
    ExtractProduct(ctx context.Context, url string) (*ProductExtractionResult, error)
}

// Orchestrator manages the AI-driven crawl loop using OpenAI function calling.
// Concrete struct (not interface) — takes FirecrawlScraper as dependency.
type Orchestrator struct {
    client  *openai.Client
    model   string
    scraper FirecrawlScraper
}
```

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

Returns to orchestrator: page title + list of link URLs. The Firecrawl `links` format returns bare URLs without anchor text — the AI classifies links based on URL patterns alone. Example:
```
Page: "Electronics - Best Buy"
Links (4 found):
- https://shop.com/product/iphone-15
- https://shop.com/product/samsung-galaxy
- https://shop.com/category/laptops
- https://shop.com/electronics?page=2
```

### extract_and_save_product(url)

Extracts structured product data from a single product page using Firecrawl's LLM, validates it, and saves it in one step.

OpenAI function schema:
```json
{
  "name": "extract_and_save_product",
  "description": "Extract product data from a product page URL and save it. Returns extraction result and progress.",
  "parameters": {
    "type": "object",
    "properties": {
      "url": { "type": "string", "description": "URL of a product page" }
    },
    "required": ["url"]
  }
}
```

Performs in one step:
1. Checks visited-URL dedup (returns early if already visited)
2. Marks URL as visited, increments scrape count
3. Calls `FirecrawlScraper.ExtractProduct(ctx, url)`
4. If product found: validates via `ValidateProducts`, dedup-checks name/URL, saves to `ToolHandlers`
5. Returns combined result: product details + progress string, or error/rejection reason

Return format:
```
Saved "Motorola Moto G35" ($444.00). Progress: 1/20 products.
Saved so far: Motorola Moto G35
```

Or on failure:
```
No product data found at https://shop.com/about. This may not be a product page — try discover_links instead.
```

Or on validation rejection:
```
Product rejected: price $0.00 out of range. Not counted. Progress: 5/20 products.
```

### get_status()

Returns current crawl progress. No parameters.

Returns: product count, target, list of saved product names.

### done()

Signals the orchestrator is finished. No parameters. Go code breaks the loop.

## System Prompt

```
You are a product discovery agent for an online price guessing game. Your job is to find and save products from an online shop.

Goal: Save at least {min_products} products from {shop_url}.

You have these tools:
- discover_links(url): Explore a page to find links. Use on listing pages, category pages, and pagination pages.
- extract_and_save_product(url): Extract product data from a product page and save it automatically. Returns the saved product info and your progress, or an error if extraction fails.
- get_status(): Check how many products you've saved and see the list.
- done(): Call when you've saved enough products or exhausted available pages.

Strategy:
1. Start by reviewing the initial page links provided below.
2. Identify which links are product pages and which are category/listing/pagination pages.
3. Use discover_links on category and pagination pages to find more products.
4. Use extract_and_save_product on product pages. You can call it on multiple URLs simultaneously — batch 3-5 product URLs per turn for efficiency.
5. Call done() when you reach {min_products} products or run out of pages to explore.

Avoid:
- Re-scraping URLs you've already visited (the system tracks this automatically).
- Following links to non-product areas (blog, about, contact, FAQ, etc.).
```

## Go Loop

### Pseudocode

```
1. Scrape initial URL via discover_links (Firecrawl ScrapeURL with links format)
2. Build messages: [system_prompt, user_message_with_initial_links]
3. Initialize: visitedURLs = {initial_url}, scrapeCount = 1, done = false
4. Loop:
   a. Check ctx.Err() → break with timeout log
   b. Check scrapeCount >= maxScrapes → break with SAFETY_CAP log
   c. Call OpenAI chat completion with messages + tool definitions
   d. Track tokens used
   e. If finish_reason == "stop" or no tool calls → break
   f. Append assistant message to conversation
   g. Dispatch tool calls (parallel for I/O, inline for local):
      - discover_links(url): call Firecrawl ScrapeURL (links)
      - extract_and_save_product(url): call Firecrawl ScrapeURL (json) + validate + save
      - get_status(): return progress string
      - done(): set done = true
   h. Append all tool results to conversation
   i. If done → break
5. Finalize crawl record with saved product count, scrape count, token usage
```

### Initial Message

Before the loop starts, Go code scrapes the initial URL and constructs the first user message:

```
Shop URL: {url}
Initial page title: "{title}"
Links found on initial page ({count}):
- {link1_url}
- {link2_url}
...

Begin exploring and saving products.
```

## Parallel Execution

When OpenAI returns multiple tool calls in one response, I/O-bound calls (`extract_and_save_product`, `discover_links`) execute concurrently.

### Execution Flow

```
OpenAI returns N tool calls
  → Partition into:
      - I/O calls (extract_and_save_product, discover_links)
      - Local calls (get_status, done)
  → For each I/O call, synchronously:
      - Check visited-URL dedup (skip if visited)
      - Mark URL as visited
      - Increment scrape count (check safety cap)
  → Launch goroutines for non-skipped I/O calls (with semaphore)
  → Execute local calls inline
  → Wait for all goroutines to complete
  → Collect results in original tool call order
  → Append all tool result messages
```

### Thread Safety

`ToolHandlers` has a `sync.Mutex`:

```go
type ToolHandlers struct {
    mu            sync.Mutex
    minProducts   int
    savedProducts []RawProduct
    seenNames     map[string]bool
    seenURLs      map[string]bool
    visitedURLs   map[string]bool
    scrapeCount   int
}
```

Locking strategy:
- **Pre-dispatch phase** (single-threaded): `MarkVisited` + `TryIncrementScrapeCount` for all I/O calls. This prevents two goroutines from both deciding to visit the same URL.
- **Goroutine phase**: Each goroutine does the Firecrawl HTTP call (no lock needed), then acquires the mutex only to call `SaveProduct` (which touches savedProducts, seenNames, seenURLs).
- **Local calls** (`get_status`, `done`): Execute inline in the main goroutine after pre-dispatch. `GetStatus` acquires the mutex.

### Concurrency Limit

A buffered channel of size 5 acts as a semaphore (`maxParallelScrapes = 5`). This prevents hammering Firecrawl's API.

### Result Ordering

Results must match tool call order for the OpenAI messages. A `results []string` slice indexed by position ensures this:

```go
results := make([]string, len(toolCalls))
var wg sync.WaitGroup

for i, tc := range toolCalls {
    if isIOCall(tc) {
        wg.Add(1)
        go func(idx int, tc ToolCall) {
            defer wg.Done()
            sem <- struct{}{}
            defer func() { <-sem }()
            results[idx] = executeIOCall(ctx, tc)
        }(i, tc)
    } else {
        results[i] = executeLocalCall(tc)
    }
}

wg.Wait()
```

## Error Handling

### Firecrawl Failures
- `discover_links` fails: return `"Failed to fetch {url}: {error}"` to orchestrator. AI skips the URL.
- `extract_and_save_product` fails or returns empty: return `"No product data found at {url}. This may not be a product page — try discover_links instead."` AI can try `discover_links` on it instead.

### Orchestrator AI Failures
- OpenAI API error mid-loop: crawl ends with products saved so far. Status `"failed"`.
- Invalid tool call arguments: return error as tool result, AI retries.
- AI loops on same URLs: visited-URL tracking returns "Already visited" without Firecrawl calls. Safety cap on scrape count prevents infinite loops.

### Duplicate Handling
- **URL dedup:** `visitedURLs` map tracks all URLs passed to `discover_links` and `extract_and_save_product`. Repeated URLs get instant response, no Firecrawl call.
- **Product dedup:** `SaveProduct` checks product name + source_url against already-saved products in current crawl. Duplicates are rejected with message.

### Validation Failures
- `SaveProduct` runs existing `ValidateProducts` logic. Rejected products get: `"Product rejected: {reason}. Not counted. Progress: 8/20."`.

### Timeout
- Context timeout covers the entire crawl (all Firecrawl + OpenAI calls). Default 300s (5 minutes).
- When context expires, Go code catches it, finalizes with products saved so far.

### Empty Results
- Initial `discover_links` returns zero links: crawl ends immediately, status `"failed"`, error `"No links found on initial page"`.
- AI calls `done()` with zero products: status `"completed"` with warning `"found only 0 products (minimum: 20)"`.

## Safety

### Scrape Count with Parallel Calls

The safety cap (`MaxScrapes`) is checked **before launching goroutines**. During the pre-dispatch phase, each I/O call increments the scrape count. If the count would exceed `MaxScrapes`, excess calls are skipped (return an error result) rather than launched.

```
Pre-dispatch:
  scrapeCount = 22, MaxScrapes = 25, batch has 5 extract calls
  → Allow first 3 (count goes to 25)
  → Skip last 2 with "Safety cap reached" message
```

### `done` in a Mixed Batch

If the AI returns `done` alongside I/O tool calls in the same response, all I/O calls execute first (their results matter for the final product count). After all goroutines complete and results are collected, `done` is processed last, setting the loop exit flag.

### Context Cancellation

Goroutines receive the parent context. If the context is cancelled mid-batch, Firecrawl calls return errors, which are reported as tool results. The main loop checks `ctx.Err()` after collecting results and breaks if cancelled.

### Error Isolation

A Firecrawl failure in one goroutine does not affect others. Each goroutine returns its own result string (success or error). The AI receives all results and can decide what to do next.

## Logging & Observability

### Log Events

| Event | When | Example |
|-------|------|---------|
| `ORCHESTRATOR_START` | Loop begins | `"Starting orchestrator loop for https://shop.com, target: 20 products"` |
| `TOOL_CALL` | AI calls any tool | `"Tool call: discover_links(https://shop.com/category/phones)"` |
| `TOOL_RESULT` | Tool returns to AI | `"discover_links returned 15 links for https://shop.com/category/phones"` |
| `TOOL_ERROR` | Tool call fails | `"extract_and_save_product failed for https://shop.com/about: no product data"` |
| `DUPLICATE_SKIP` | URL already visited | `"Skipping already-visited URL: https://shop.com/page/2"` |
| `PRODUCT_SAVED` | Product persisted | `"Saved: iPhone 15 Pro Max ($999.00). Progress: 8/20"` |
| `PRODUCT_REJECTED` | Validation failed | `"Product rejected: price $0.00 out of range"` |
| `LINKS_FOUND` | Links discovered | `"discover_links returned 15 links for https://shop.com/category/phones"` |
| `ORCHESTRATOR_DONE` | Loop finishes | `"Orchestrator finished. 22 products saved in 12 scrape calls"` |
| `SAFETY_CAP` | Max scrapes hit | `"Safety cap reached (50 scrape calls). Ending loop with 15 products"` |
| `AI_TOKENS` | After each OpenAI call | `"OpenAI call used 1,250 tokens (total this crawl: 8,400)"` |

### Token Tracking

Each OpenAI completion returns `Usage.TotalTokens`. The Go code accumulates this across the loop. `CrawlResult` and `OrchestratorResult` both have a `TotalTokensUsed int` field.

### Progress Reporting

The existing `ProgressFunc` callback fires on key events (tool calls, products saved, loop end) for CLI real-time output.

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `FIRECRAWL_API_KEY` | (required) | Firecrawl API authentication key |
| `FIRECRAWL_API_URL` | `https://api.firecrawl.dev` | Firecrawl API base URL (for self-hosted) |
| `FIRECRAWL_MAX_SCRAPES` | `50` | Max Firecrawl scrape calls per crawl (safety cap) |
| `CRAWL_TIMEOUT` | `300` | Crawl timeout in seconds |

### CrawlConfig Struct

```go
type CrawlConfig struct {
    URL         string
    Timeout     time.Duration  // default 300s
    MinProducts int            // default 20
    MaxScrapes  int            // default 50
    Verbose     bool
}
```

## Files

| File | Purpose |
|------|---------|
| `crawler.go` | Top-level crawl pipeline: shop/crawl record management, initial scrape, orchestrator invocation, product persistence, finalization |
| `orchestrator.go` | AI-driven loop: OpenAI calls, parallel tool dispatch, system prompt, tool definitions |
| `tool_handlers.go` | Thread-safe crawl state: visited URLs, saved products, scrape count, deduplication |
| `firecrawl_scraper.go` | `FirecrawlScraper` interface + `FirecrawlClient` implementation using firecrawl-go SDK |
| `validator.go` | Product validation (name, price, image URL) and deduplication by normalized name |
| `logger.go` | Per-crawl file logging with timestamps and log levels |
