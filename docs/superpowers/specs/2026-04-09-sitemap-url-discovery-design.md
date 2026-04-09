# Sitemap-based URL Discovery for Crawler

**Date:** 2026-04-09
**Status:** Approved

## Problem

The current crawler uses BFS link discovery starting from a seed URL, with AI fallback when few links are found. This approach is:
- **Slow** - BFS with 1 req/s rate limiting takes many iterations to reach product pages
- **Expensive** - AI is invoked for link discovery, consuming tokens
- **Incomplete** - BFS may miss product pages not directly linked from the seed URL
- **Fragile** - regex-based `<a href>` extraction can miss JS-generated links

Most e-commerce shops publish `sitemap.xml` (required for SEO) which contains direct URLs to product pages. Using sitemap as a primary URL discovery source would be faster, cheaper, and more complete.

## Design

### Approach: New `sitemap.go` component (Approach A)

A new file `backend/internal/crawler/sitemap.go` with its own tests, following the existing pattern of one file per component (`fetcher.go`, `extractor.go`, `validator.go`). No changes to existing interfaces.

### New Component: `sitemap.go`

**XML types** for parsing sitemap protocol (sitemaps.org/protocol.html):
- `sitemapIndex` - `<sitemapindex>` with nested `<sitemap>` entries
- `urlSet` - `<urlset>` with `<url>` entries containing `<loc>`

**`FetchSitemapURLs(ctx context.Context, fetcher Fetcher, baseURL string) ([]string, error)`**
Main entry point:
1. Fetches `{baseURL}/sitemap.xml` using existing `Fetcher.Fetch()`. Note: `Fetcher.Fetch()` returns an error for non-200 responses (e.g., 404), so fetch errors are treated as "no sitemap" rather than failures.
2. Tries to parse as `<sitemapindex>` - if so, fetches up to 5 sub-sitemaps (errors on individual sub-sitemaps are logged but skipped)
3. Tries to parse as `<urlset>` - collects URLs
4. If `/sitemap.xml` returns error, tries to find Sitemap URL from `robots.txt` (parses `Sitemap:` directives from the raw text - requires a separate fetch of robots.txt or reuse of already-fetched content)
5. Returns unique same-host URLs (filtered inside this function using `url.Parse` to compare hosts), capped at 500
6. If sitemap doesn't exist or is empty - returns `nil, nil` (no error)

**`SampleURLs(urls []string, n int) []string`**
Fisher-Yates shuffle, returns `n` random URLs from the list. If `len(urls) <= n`, returns all. Ensures diversity of products across categories.

### Integration with `crawler.go`

New step between "3. Check robots.txt" and "4. Crawl pages":

```go
// 3.5 Try sitemap-based URL discovery
sitemapURLs, err := FetchSitemapURLs(ctx, c.fetcher, cfg.URL)
if err != nil {
    logger.Log("FETCH", fmt.Sprintf("Sitemap fetch failed: %v", err))
} else if len(sitemapURLs) > 0 {
    logger.Log("FETCH", fmt.Sprintf("Found %d URLs from sitemap", len(sitemapURLs)))
    report("sitemap", fmt.Sprintf("Found %d URLs in sitemap", len(sitemapURLs)))
    sampled := SampleURLs(sitemapURLs, 100)
    logger.Log("FETCH", fmt.Sprintf("Sampled %d URLs from sitemap for crawling", len(sampled)))
    toVisit = append(sampled, toVisit...)
}
```

Sitemap URLs are prepended to `toVisit`, giving them priority over BFS-discovered links. Existing BFS logic (link extraction, AI fallback) still works as fallback.

### Error Handling

| Scenario | Behavior |
|----------|----------|
| Sitemap doesn't exist (404) | Return `nil, nil`, fallback to BFS |
| Sitemap returns HTML (redirect) | XML parsing fails, return `nil, nil` |
| Sitemap index with many sub-sitemaps | Parse max 5 sub-sitemaps |
| Timeout | Uses same `ctx` as crawler, respects global timeout |
| Rate limiting | Fetches go through `Fetcher.Fetch()` with rate limiter |
| Invalid XML | Return `nil, nil` |
| Empty sitemap | Return `nil, nil` |

### Tests (`sitemap_test.go`)

Using `httptest` servers:
1. Parse simple `<urlset>` - returns URLs
2. Parse `<sitemapindex>` - fetches sub-sitemap and returns URLs
3. Sitemap doesn't exist (404) - returns nil, nil
4. Invalid XML - returns nil, nil
5. Empty sitemap - returns nil, nil
6. Sitemap URL from robots.txt - fetches from declared location
7. `SampleURLs` - samples correct count, returns all if n > len(urls)

Existing `crawler_test.go` tests remain unchanged - mock fetcher returns 404 on `/sitemap.xml`.

### Files Changed

- **New:** `backend/internal/crawler/sitemap.go`
- **New:** `backend/internal/crawler/sitemap_test.go`
- **Modified:** `backend/internal/crawler/crawler.go` (~10 lines added)

### What This Does NOT Change

- `Fetcher` interface - unchanged
- `AIClient` interface - unchanged
- Existing BFS logic - unchanged, serves as fallback
- Existing tests - unchanged
- CLI flags - no new flags needed
