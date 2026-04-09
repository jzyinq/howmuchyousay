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
