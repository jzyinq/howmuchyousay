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
					Markdown: "# Laptop Dell XPS 15\n\nCompare prices and specs\n\n5999.99 PLN | 4999.99 PLN refurbished\n\nAdd to cart\n\nThe best laptop for professionals.",
					Metadata: map[string]string{
						"title":   "Laptop Dell XPS 15 - Shop",
						"ogImage": "https://img.com/dell.jpg",
					},
				},
				{
					URL:      "https://shop.com/products/iphone",
					Markdown: "# iPhone 15 Pro\n\nPrice: 5499 PLN\n\nBuy now\n\nThe latest iPhone with titanium design and advanced camera system.",
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

	// Page with ogTitle + exactly one price in markdown — no AI needed
	mockFC := &mockFirecrawlCrawler{
		crawlSiteFn: func(ctx context.Context, siteURL string, cfg crawler.CrawlConfig) ([]crawler.PageResult, error) {
			return []crawler.PageResult{
				{
					URL:      "https://shop.com/product/phone",
					Markdown: "# iPhone 15 Pro\n\nBest phone ever with amazing camera and titanium design. Available now in multiple colors.\n\nPrice: 5499.00 PLN",
					Metadata: map[string]string{
						"ogTitle": "iPhone 15 Pro",
						"ogImage": "https://img.com/iphone.jpg",
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
					Markdown: "# DB Test Product\n\nPrice: 99.99 PLN\n\nGreat product for testing database persistence with metadata extraction.",
					Metadata: map[string]string{
						"ogTitle": "DB Test Product",
						"ogImage": "https://img.com/db.jpg",
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
