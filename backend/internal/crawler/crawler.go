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
		scrapes := 1
		aiReqs := 0
		if orchResult != nil {
			scrapes += orchResult.ScrapeCount
			aiReqs = orchResult.AIRequestCount
		}
		if finErr := c.crawlStore.Finish(ctx, crawl.ID, models.CrawlStatusFailed, 0, scrapes, aiReqs, &errMsg); finErr != nil {
			logger.Log("ERROR", fmt.Sprintf("Failed to finish crawl record: %v", finErr))
		}
		return &CrawlResult{
			CrawlID:     crawl.ID,
			ShopID:      shop.ID,
			ScrapeCount: scrapes,
			Duration:    time.Since(start),
			LogFilePath: logger.FilePath(),
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
