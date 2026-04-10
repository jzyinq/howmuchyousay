# Firecrawl Integration Design

**Date:** 2026-04-10
**Status:** Approved

## Summary

Replace the custom HTTP fetching, BFS crawling, sitemap discovery, and robots.txt handling in the crawler with Firecrawl's `/v2/crawl` API. Keep the existing GPT-5 mini product extraction and validation pipeline, but feed it clean markdown instead of raw HTML.

## Motivation

The current crawler maintains ~500 lines of fetching, BFS navigation, sitemap parsing, robots.txt checking, and rate limiting code. Firecrawl handles all of this (plus JS rendering, proxy rotation, anti-bot evasion, and caching) as a service. The markdown output it produces is significantly cleaner input for LLM-based product extraction than raw HTML -- fewer tokens, better accuracy.

## Architecture

```
CLI (--url, --timeout, --min-products)
  │
  ▼
Crawler.Run()
  │
  ├── 1. Find/create shop in DB
  ├── 2. Create crawl record (status: pending → in_progress)
  ├── 3. Start Firecrawl /v2/crawl job (via Go SDK)
  │     └── CrawlParams: MaxDepth, Limit, ExcludePaths
  ├── 4. Poll for completion (SDK auto-polls)
  │     └── Progress callback: pages completed / total
  ├── 5. For each returned page:
  │     ├── a. Check Firecrawl metadata for product signals
  │     ├── b. Send markdown to GPT-5 mini for product extraction
  │     └── c. Accumulate RawProducts
  ├── 6. Validate & deduplicate (existing validator.go)
  ├── 7. Save products to DB (existing product_store.go)
  └── 8. Finish crawl record (status: completed)
```

## Components

### 1. Firecrawl Client (`firecrawl_client.go`)

Thin wrapper around the `firecrawl-go/v2` SDK providing a testable interface:

```go
type FirecrawlCrawler interface {
    CrawlSite(ctx context.Context, url string, cfg CrawlConfig) ([]PageResult, error)
}

type PageResult struct {
    URL      string
    Markdown string
    Metadata map[string]string // title, description, ogImage, etc.
}
```

Implementation:
- Initialize with `firecrawl.NewFirecrawlApp(apiKey, apiURL)`
- Call `CrawlUrl(url, params, idempotencyKey)` which blocks until completion
- Map SDK response structs to our `PageResult` type
- Respect `context.Context` for timeout/cancellation

**CrawlParams defaults:**
- `MaxDepth`: 3 (covers category → subcategory → product page patterns)
- `Limit`: `minProducts * 3` (tunable heuristic -- not every page has products; adjust multiplier based on observed hit rates)
- `ExcludePaths`: `["blog/*", "about/*", "contact/*", "faq/*", "help/*", "terms/*", "privacy/*", "career*"]`
- `Formats`: `["markdown"]`

### 2. AI Extraction Adaptation (`ai_client.go`)

The `ExtractProducts` method prompt changes from:

> "Extract products from this HTML page..."

to:

> "Extract products from this markdown content of a web page..."

The markdown input is naturally smaller than the 100K-char truncated HTML, so the token limit handling simplifies. The response format (JSON array of `{name, price, image_url}`) stays the same.

The `ExtractLinks` method is **removed** -- Firecrawl handles link discovery.

### 3. Structured Data Extraction (`extractor.go`)

The current regex-based JSON-LD and Open Graph extraction operates on raw HTML. With Firecrawl:

- **Open Graph data**: Available directly in Firecrawl's `metadata` fields (`ogTitle`, `ogImage`, `ogDescription`). No HTML parsing needed.
- **JSON-LD**: Not present in markdown output. Firecrawl may not preserve `<script type="application/ld+json">` blocks. Structured data extraction from HTML is removed.
- **New extraction path**: Check Firecrawl metadata for product-like signals (e.g., metadata contains title that looks like a product name). This is supplementary -- the primary extraction path is AI on markdown.

`ExtractProducts()` is refactored to work with `PageResult` instead of raw HTML. `ExtractLinks()` is removed.

### 4. Crawler Orchestrator (`crawler.go`)

The `RunWithProgress` method is heavily simplified:

**Removed:**
- BFS crawl loop (`toVisit` queue, `visited` set, link following)
- Sitemap URL discovery and sampling
- Robots.txt checking
- Per-page HTTP fetching
- AI link discovery fallback
- Rate limiting coordination

**Kept:**
- Shop find/create
- Crawl record lifecycle (create → in_progress → completed/failed)
- Per-crawl logger
- Product validation and deduplication
- Database persistence
- Progress reporting

**New:**
- Single call to `FirecrawlCrawler.CrawlSite()` replaces the entire BFS loop
- Iteration over `[]PageResult` to extract products from each page

### 5. Configuration (`config.go`)

**Added:**
- `FIRECRAWL_API_KEY` (string, required) -- API authentication key
- `FIRECRAWL_API_URL` (string, optional, default: `https://api.firecrawl.dev`) -- API base URL, useful for self-hosted Firecrawl or testing
- `FIRECRAWL_MAX_DEPTH` (int, optional, default: 3) -- Maximum crawl depth

**Unchanged:**
- `DATABASE_URL`, `OPENAI_API_KEY`, `OPENAI_MODEL`, `LOG_DIR`, `SERVER_PORT`

### 6. Logger Updates (`logger.go`)

**Removed log levels:**
- `FETCH` (no individual HTTP requests)

**Added log levels:**
- `FIRECRAWL_START` -- crawl job initiated, includes job ID
- `FIRECRAWL_PROGRESS` -- pages completed / total (periodic)
- `FIRECRAWL_COMPLETE` -- crawl job finished, total pages returned

**Kept:**
- `AI_REQUEST`, `AI_RESPONSE`, `PRODUCT_FOUND`, `VALIDATION`, `ERROR`, `NAVIGATE`, `PARSE`

### 7. CLI (`cmd/crawler/main.go`)

Flags unchanged: `--url`, `--timeout`, `--min-products`, `--verbose`

Setup changes:
- Create `FirecrawlClient` instead of `HTTPFetcher`
- Remove sitemap-related setup
- Pass `FirecrawlCrawler` to `Crawler` constructor instead of `Fetcher`

## Files Changed

### Deleted
| File | Reason |
|------|--------|
| `fetcher.go` | Replaced by Firecrawl API |
| `fetcher_test.go` | Tests for deleted code |
| `sitemap.go` | Replaced by Firecrawl's crawl discovery |
| `sitemap_test.go` | Tests for deleted code |

### Heavily Modified
| File | Changes |
|------|---------|
| `crawler.go` | Remove BFS loop, replace with Firecrawl crawl + iterate results |
| `crawler_test.go` | Rewrite integration tests with mock `FirecrawlCrawler` |
| `extractor.go` | Adapt to work with `PageResult` (markdown + metadata) instead of HTML |
| `extractor_test.go` | Update test fixtures for markdown/metadata input |
| `ai_client.go` | Update prompts for markdown input, remove `ExtractLinks` |
| `ai_client_test.go` | Update tests for new prompts, remove link extraction tests |
| `logger.go` | Add new log levels, remove `FETCH` |
| `logger_test.go` | Update for new log levels |
| `config/config.go` | Add Firecrawl config fields |
| `cmd/crawler/main.go` | Use `FirecrawlClient` instead of `HTTPFetcher` |

### Added
| File | Purpose |
|------|---------|
| `firecrawl_client.go` | `FirecrawlCrawler` interface + SDK-based implementation |
| `firecrawl_client_test.go` | Unit tests with mocked HTTP responses |

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Firecrawl API auth error (401) | Crawl fails immediately, error message stored |
| Firecrawl rate limit (429) | SDK retries automatically; if exhausted, crawl fails |
| Firecrawl server error (500) | Crawl fails, error message stored |
| Context timeout during crawl | Mark crawl as failed, attempt to use any partial results available |
| Firecrawl returns 0 pages | Crawl fails with descriptive error |
| AI extraction fails for a page | Skip page, continue with remaining pages, log error |
| No products found across all pages | Crawl completes with `products_found: 0` |

## Dependencies

**Added:**
- `github.com/mendableai/firecrawl-go/v2` -- Official Firecrawl Go SDK

**Removed:**
- `golang.org/x/time/rate` -- Rate limiting no longer needed (Firecrawl handles it)

## Testing Strategy

- **Unit tests** for `firecrawl_client.go`: Mock HTTP server returning Firecrawl-shaped JSON responses
- **Unit tests** for updated `ai_client.go`: Verify markdown-based prompts work correctly
- **Unit tests** for updated `extractor.go`: Test extraction from `PageResult` metadata
- **Integration tests** for `crawler.go`: Mock `FirecrawlCrawler` interface, verify full pipeline from crawl results to DB persistence
- **Existing validator tests**: Unchanged (validator doesn't care about input source)
- **Existing logger tests**: Updated for new log levels

## Migration Notes

- Existing crawl records in the database are unaffected (schema unchanged)
- Environment must have `FIRECRAWL_API_KEY` set for the crawler to work
- `.env.example` updated with the new environment variable
- The `OPENAI_API_KEY` is still required for GPT-5 mini product extraction
