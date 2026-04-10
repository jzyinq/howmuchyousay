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
