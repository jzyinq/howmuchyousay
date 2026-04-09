# Sitemap-based URL Discovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add sitemap.xml-based URL discovery to the crawler, giving sitemap URLs priority over BFS link discovery with a random sample of 100 URLs from up to 500 collected.

**Architecture:** New `sitemap.go` component in `backend/internal/crawler/` handles fetching and parsing sitemap XML (both `<urlset>` and `<sitemapindex>`), with fallback to `robots.txt` Sitemap directives. The crawler orchestrator (`crawler.go`) calls this before the BFS loop, prepending sampled URLs to the visit queue. No interface changes.

**Tech Stack:** Go 1.23+, `encoding/xml`, `net/url`, `math/rand/v2`, existing `Fetcher` interface, `httptest` for testing

**Plan sequence:** This is an enhancement to Plan 2 (Crawler CLI & AI Agent), adding sitemap discovery to the existing crawl pipeline.

**Prerequisite:** Plan 2 must be fully implemented (all crawler components).

**Spec:** `docs/superpowers/specs/2026-04-09-sitemap-url-discovery-design.md`

---

## File Structure

```
backend/internal/crawler/
├── sitemap.go           # NEW: Sitemap fetching, XML parsing, URL sampling
├── sitemap_test.go      # NEW: Tests with httptest servers
├── crawler.go           # MODIFIED: Add sitemap discovery step before BFS loop
├── fetcher.go           # UNCHANGED
├── extractor.go         # UNCHANGED
├── validator.go         # UNCHANGED
├── ai_client.go         # UNCHANGED
├── logger.go            # UNCHANGED
└── crawler_test.go      # UNCHANGED
```

- **sitemap.go** — XML types for sitemap protocol, `FetchSitemapURLs()` entry point, `parseSitemapXML()` for parsing, `extractSitemapFromRobotsTxt()` for robots.txt fallback, `SampleURLs()` for random sampling
- **sitemap_test.go** — 7 tests covering: urlset parsing, sitemapindex parsing, 404 handling, invalid XML, empty sitemap, robots.txt fallback, SampleURLs behavior
- **crawler.go** — ~10 lines added between robots.txt check and BFS loop

---

### Task 1: SampleURLs Function and XML Types

**Files:**
- Create: `backend/internal/crawler/sitemap.go`

Start with the simplest piece: the `SampleURLs` helper function and XML type definitions. These have no dependencies on the rest of the sitemap code.

- [ ] **Step 1: Write failing tests for SampleURLs**

Create `backend/internal/crawler/sitemap_test.go`:

```go
package crawler_test

import (
	"testing"

	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/stretchr/testify/assert"
)

func TestSampleURLs_ReturnsAllWhenFewerThanN(t *testing.T) {
	urls := []string{"http://a.com/1", "http://a.com/2", "http://a.com/3"}
	result := crawler.SampleURLs(urls, 10)
	assert.Len(t, result, 3)
	// All original URLs should be present
	assert.ElementsMatch(t, urls, result)
}

func TestSampleURLs_ReturnsExactlyN(t *testing.T) {
	urls := make([]string, 200)
	for i := range urls {
		urls[i] = fmt.Sprintf("http://a.com/%d", i)
	}
	result := crawler.SampleURLs(urls, 100)
	assert.Len(t, result, 100)

	// All returned URLs should be from the original list
	urlSet := make(map[string]bool)
	for _, u := range urls {
		urlSet[u] = true
	}
	for _, u := range result {
		assert.True(t, urlSet[u], "sampled URL should be from original list")
	}
}

func TestSampleURLs_EmptyInput(t *testing.T) {
	result := crawler.SampleURLs(nil, 100)
	assert.Empty(t, result)
}

func TestSampleURLs_ZeroN(t *testing.T) {
	urls := []string{"http://a.com/1"}
	result := crawler.SampleURLs(urls, 0)
	assert.Empty(t, result)
}
```

Note: Add `"fmt"` to the import block (needed for `Sprintf` in `TestSampleURLs_ReturnsExactlyN`).

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/crawler/ -v -run "TestSampleURLs" -count=1`
Expected: FAIL — `crawler.SampleURLs` undefined.

- [ ] **Step 3: Implement SampleURLs and XML types**

Create `backend/internal/crawler/sitemap.go`:

```go
package crawler

import (
	"context"
	"encoding/xml"
	"math/rand/v2"
	"net/url"
	"strings"
)

// maxSitemapURLs is the maximum number of URLs to collect from sitemaps.
const maxSitemapURLs = 500

// maxSubSitemaps is the maximum number of sub-sitemaps to fetch from a sitemap index.
const maxSubSitemaps = 5

// --- XML types for sitemap protocol (sitemaps.org/protocol.html) ---

// sitemapIndex represents a <sitemapindex> element containing nested sitemaps.
type sitemapIndex struct {
	XMLName  xml.Name       `xml:"sitemapindex"`
	Sitemaps []sitemapEntry `xml:"sitemap"`
}

// sitemapEntry represents a single <sitemap> within a sitemap index.
type sitemapEntry struct {
	Loc string `xml:"loc"`
}

// urlSet represents a <urlset> element containing page URLs.
type urlSet struct {
	XMLName xml.Name   `xml:"urlset"`
	URLs    []urlEntry `xml:"url"`
}

// urlEntry represents a single <url> within a urlset.
type urlEntry struct {
	Loc string `xml:"loc"`
}

// SampleURLs returns a random sample of n URLs from the given slice.
// If len(urls) <= n, returns a copy of the entire slice.
// Uses Fisher-Yates shuffle for unbiased sampling.
func SampleURLs(urls []string, n int) []string {
	if len(urls) == 0 || n <= 0 {
		return nil
	}

	if len(urls) <= n {
		// Return a copy to avoid mutating the original
		result := make([]string, len(urls))
		copy(result, urls)
		return result
	}

	// Fisher-Yates shuffle on a copy, then take first n
	shuffled := make([]string, len(urls))
	copy(shuffled, urls)
	for i := len(shuffled) - 1; i > 0; i-- {
		j := rand.IntN(i + 1)
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}

	return shuffled[:n]
}
```

Note: `FetchSitemapURLs` and helper functions will be added in Task 2. The `context`, `net/url`, and `strings` imports will be used there — for now, the compiler may warn about unused imports. To avoid this, we add only the imports needed for this step:

Replace the import block with:

```go
import (
	"encoding/xml"
	"math/rand/v2"
)
```

The full import block will be restored in Task 2.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/crawler/ -v -run "TestSampleURLs" -count=1`
Expected: All 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat(crawler): add SampleURLs helper and sitemap XML types"
```

---

### Task 2: FetchSitemapURLs — Core Sitemap Parsing

**Files:**
- Modify: `backend/internal/crawler/sitemap.go`
- Modify: `backend/internal/crawler/sitemap_test.go`

Add the main `FetchSitemapURLs` function and its internal helpers for parsing `<urlset>` and `<sitemapindex>`.

- [ ] **Step 1: Write failing tests for FetchSitemapURLs**

Add to `backend/internal/crawler/sitemap_test.go` (below the existing SampleURLs tests):

```go
import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchSitemapURLs_SimpleURLSet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sitemap.xml" {
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>` + "http://" + r.Host + `/products/1</loc></url>
  <url><loc>` + "http://" + r.Host + `/products/2</loc></url>
  <url><loc>` + "http://" + r.Host + `/products/3</loc></url>
</urlset>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	fetcher := crawler.NewHTTPFetcher(5 * time.Second)
	ctx := context.Background()

	urls, err := crawler.FetchSitemapURLs(ctx, fetcher, server.URL)
	require.NoError(t, err)
	assert.Len(t, urls, 3)
	assert.Contains(t, urls, server.URL+"/products/1")
	assert.Contains(t, urls, server.URL+"/products/2")
	assert.Contains(t, urls, server.URL+"/products/3")
}

func TestFetchSitemapURLs_SitemapIndex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>` + "http://" + r.Host + `/sitemap-products.xml</loc></sitemap>
</sitemapindex>`))
		case "/sitemap-products.xml":
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>` + "http://" + r.Host + `/product/a</loc></url>
  <url><loc>` + "http://" + r.Host + `/product/b</loc></url>
</urlset>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	fetcher := crawler.NewHTTPFetcher(5 * time.Second)
	ctx := context.Background()

	urls, err := crawler.FetchSitemapURLs(ctx, fetcher, server.URL)
	require.NoError(t, err)
	assert.Len(t, urls, 2)
	assert.Contains(t, urls, server.URL+"/product/a")
	assert.Contains(t, urls, server.URL+"/product/b")
}

func TestFetchSitemapURLs_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	fetcher := crawler.NewHTTPFetcher(5 * time.Second)
	ctx := context.Background()

	urls, err := crawler.FetchSitemapURLs(ctx, fetcher, server.URL)
	assert.NoError(t, err)
	assert.Nil(t, urls)
}

func TestFetchSitemapURLs_InvalidXML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sitemap.xml" {
			w.Write([]byte(`<html><body>Not XML sitemap</body></html>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	fetcher := crawler.NewHTTPFetcher(5 * time.Second)
	ctx := context.Background()

	urls, err := crawler.FetchSitemapURLs(ctx, fetcher, server.URL)
	assert.NoError(t, err)
	assert.Nil(t, urls)
}

func TestFetchSitemapURLs_EmptySitemap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sitemap.xml" {
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
</urlset>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	fetcher := crawler.NewHTTPFetcher(5 * time.Second)
	ctx := context.Background()

	urls, err := crawler.FetchSitemapURLs(ctx, fetcher, server.URL)
	assert.NoError(t, err)
	assert.Nil(t, urls)
}
```

Make sure the import block at the top of the file includes all needed imports:

```go
import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/crawler/ -v -run "TestFetchSitemapURLs" -count=1`
Expected: FAIL — `crawler.FetchSitemapURLs` undefined.

- [ ] **Step 3: Implement FetchSitemapURLs**

Modify `backend/internal/crawler/sitemap.go` — replace the import block and add functions after `SampleURLs`:

Update the import block to:

```go
import (
	"context"
	"encoding/xml"
	"math/rand/v2"
	"net/url"
	"strings"
)
```

Add after the `SampleURLs` function:

```go
// FetchSitemapURLs tries to discover URLs from the site's sitemap.xml.
// Returns collected URLs (up to maxSitemapURLs) or nil if no sitemap found.
// Errors from fetching are treated as "no sitemap" (returns nil, nil).
func FetchSitemapURLs(ctx context.Context, fetcher Fetcher, baseURL string) ([]string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, nil
	}

	sitemapURL := parsed.Scheme + "://" + parsed.Host + "/sitemap.xml"

	urls, err := fetchAndParseSitemap(ctx, fetcher, sitemapURL, parsed.Host)
	if err == nil && len(urls) > 0 {
		return urls, nil
	}

	// Fallback: try to find Sitemap directive in robots.txt
	robotsURL := parsed.Scheme + "://" + parsed.Host + "/robots.txt"
	robotsBody, err := fetcher.Fetch(ctx, robotsURL)
	if err != nil {
		return nil, nil
	}

	sitemapLocs := extractSitemapFromRobotsTxt(robotsBody)
	if len(sitemapLocs) == 0 {
		return nil, nil
	}

	var allURLs []string
	for _, loc := range sitemapLocs {
		if len(allURLs) >= maxSitemapURLs {
			break
		}
		found, err := fetchAndParseSitemap(ctx, fetcher, loc, parsed.Host)
		if err != nil {
			continue
		}
		allURLs = append(allURLs, found...)
	}

	if len(allURLs) == 0 {
		return nil, nil
	}
	if len(allURLs) > maxSitemapURLs {
		allURLs = allURLs[:maxSitemapURLs]
	}
	return allURLs, nil
}

// fetchAndParseSitemap fetches a single sitemap URL and parses it.
// Handles both <sitemapindex> and <urlset> formats.
// Returns same-host URLs only, capped at maxSitemapURLs.
func fetchAndParseSitemap(ctx context.Context, fetcher Fetcher, sitemapURL string, allowedHost string) ([]string, error) {
	body, err := fetcher.Fetch(ctx, sitemapURL)
	if err != nil {
		return nil, err
	}

	return parseSitemapXML(ctx, fetcher, body, allowedHost)
}

// parseSitemapXML parses XML content as either a sitemap index or a urlset.
func parseSitemapXML(ctx context.Context, fetcher Fetcher, xmlContent string, allowedHost string) ([]string, error) {
	// Try <sitemapindex> first
	var index sitemapIndex
	if err := xml.Unmarshal([]byte(xmlContent), &index); err == nil && len(index.Sitemaps) > 0 {
		var allURLs []string
		count := 0
		for _, s := range index.Sitemaps {
			if count >= maxSubSitemaps {
				break
			}
			if s.Loc == "" {
				continue
			}
			count++
			subURLs, err := fetchAndParseSitemap(ctx, fetcher, s.Loc, allowedHost)
			if err != nil {
				continue
			}
			allURLs = append(allURLs, subURLs...)
			if len(allURLs) >= maxSitemapURLs {
				allURLs = allURLs[:maxSitemapURLs]
				break
			}
		}
		return allURLs, nil
	}

	// Try <urlset>
	var us urlSet
	if err := xml.Unmarshal([]byte(xmlContent), &us); err == nil && len(us.URLs) > 0 {
		var urls []string
		seen := make(map[string]bool)
		for _, u := range us.URLs {
			if u.Loc == "" {
				continue
			}
			parsedLoc, err := url.Parse(u.Loc)
			if err != nil {
				continue
			}
			// Same-host filter
			if parsedLoc.Host != allowedHost {
				continue
			}
			if seen[u.Loc] {
				continue
			}
			seen[u.Loc] = true
			urls = append(urls, u.Loc)
			if len(urls) >= maxSitemapURLs {
				break
			}
		}
		return urls, nil
	}

	return nil, nil
}

// extractSitemapFromRobotsTxt extracts Sitemap: directive URLs from robots.txt content.
func extractSitemapFromRobotsTxt(content string) []string {
	var sitemaps []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "sitemap:") {
			loc := strings.TrimSpace(line[len("sitemap:"):])
			if loc != "" {
				sitemaps = append(sitemaps, loc)
			}
		}
	}
	return sitemaps
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/crawler/ -v -run "TestFetchSitemapURLs|TestSampleURLs" -count=1`
Expected: All 9 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat(crawler): add FetchSitemapURLs with urlset and sitemapindex parsing"
```

---

### Task 3: Robots.txt Sitemap Fallback Test

**Files:**
- Modify: `backend/internal/crawler/sitemap_test.go`

Add a test for the robots.txt Sitemap directive fallback path.

- [ ] **Step 1: Write the test**

Add to `backend/internal/crawler/sitemap_test.go`:

```go
func TestFetchSitemapURLs_RobotsTxtFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sitemap.xml":
			// Main sitemap doesn't exist
			w.WriteHeader(http.StatusNotFound)
		case "/robots.txt":
			w.Write([]byte("User-agent: *\nAllow: /\nSitemap: http://" + r.Host + "/custom-sitemap.xml\n"))
		case "/custom-sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>` + "http://" + r.Host + `/item/x</loc></url>
  <url><loc>` + "http://" + r.Host + `/item/y</loc></url>
</urlset>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	fetcher := crawler.NewHTTPFetcher(5 * time.Second)
	ctx := context.Background()

	urls, err := crawler.FetchSitemapURLs(ctx, fetcher, server.URL)
	require.NoError(t, err)
	assert.Len(t, urls, 2)
	assert.Contains(t, urls, server.URL+"/item/x")
	assert.Contains(t, urls, server.URL+"/item/y")
}
```

- [ ] **Step 2: Run test to verify it passes**

Run: `cd backend && go test ./internal/crawler/ -v -run "TestFetchSitemapURLs_RobotsTxtFallback" -count=1`
Expected: PASS (implementation from Task 2 already handles this path).

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "test(crawler): add robots.txt sitemap fallback test"
```

---

### Task 4: Integrate Sitemap Discovery into Crawler Orchestrator

**Files:**
- Modify: `backend/internal/crawler/crawler.go:162-169`

Add sitemap URL discovery between the robots.txt check and the BFS loop.

- [ ] **Step 1: Add sitemap discovery step**

In `backend/internal/crawler/crawler.go`, find the section between the robots.txt check and the BFS loop setup. Insert the sitemap discovery code between line 162 (`}` closing the robots.txt disallowed block) and line 164 (`// 4. Crawl pages and collect products`).

Replace:

```go
	// 4. Crawl pages and collect products
	var allProducts []RawProduct
	pagesVisited := 0
	aiRequestsCount := 0
	visited := make(map[string]bool)
	toVisit := []string{cfg.URL}
```

With:

```go
	// 3.5 Try sitemap-based URL discovery
	sitemapURLs, sitemapErr := FetchSitemapURLs(ctx, c.fetcher, cfg.URL)
	if sitemapErr != nil {
		logger.Log("FETCH", fmt.Sprintf("Sitemap fetch failed: %v", sitemapErr))
	}

	// 4. Crawl pages and collect products
	var allProducts []RawProduct
	pagesVisited := 0
	aiRequestsCount := 0
	visited := make(map[string]bool)
	toVisit := []string{cfg.URL}

	if len(sitemapURLs) > 0 {
		logger.Log("FETCH", fmt.Sprintf("Found %d URLs from sitemap", len(sitemapURLs)))
		report("sitemap", fmt.Sprintf("Found %d URLs in sitemap", len(sitemapURLs)))
		sampled := SampleURLs(sitemapURLs, 100)
		logger.Log("FETCH", fmt.Sprintf("Sampled %d URLs from sitemap for crawling", len(sampled)))
		toVisit = append(sampled, toVisit...)
	}
```

- [ ] **Step 2: Run existing crawler tests to verify no regression**

Run: `cd backend && go test ./internal/crawler/ -v -run "TestCrawler_" -count=1 -timeout 60s`

Note: These tests use mock fetchers that return errors for unknown URLs (including `/sitemap.xml`), so sitemap discovery will gracefully return nil and the BFS path will execute as before.

If tests require a database, run:
`cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/crawler/ -v -run "TestCrawler_" -count=1 -timeout 60s`

Expected: All existing crawler tests PASS.

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "feat(crawler): integrate sitemap URL discovery into crawl pipeline"
```

---

### Task 5: Full Test Suite & Vet

- [ ] **Step 1: Run all sitemap tests**

Run: `cd backend && go test ./internal/crawler/ -v -run "TestFetchSitemapURLs|TestSampleURLs" -count=1`
Expected: All 10 tests PASS.

- [ ] **Step 2: Run all crawler tests**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/crawler/ -v -count=1 -timeout 120s`
Expected: All tests PASS (approximately 30+ tests).

- [ ] **Step 3: Run all project tests**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./... -v -count=1 -timeout 120s`
Expected: All tests PASS.

- [ ] **Step 4: Run go vet**

Run: `cd backend && go vet ./...`
Expected: No issues.

- [ ] **Step 5: Build both binaries**

Run: `cd backend && go build ./cmd/server/ && go build ./cmd/crawler/`
Expected: Both binaries build successfully.
