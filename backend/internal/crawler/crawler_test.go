package crawler_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/jzy/howmuchyousay/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock AI Client ---

type mockAIClient struct {
	extractProductsFn func(ctx context.Context, html string, pageURL string) ([]crawler.RawProduct, int, error)
	extractLinksFn    func(ctx context.Context, html string, baseURL string) ([]string, int, error)
}

func (m *mockAIClient) ExtractProducts(ctx context.Context, html string, pageURL string) ([]crawler.RawProduct, int, error) {
	if m.extractProductsFn != nil {
		return m.extractProductsFn(ctx, html, pageURL)
	}
	return nil, 0, nil
}

func (m *mockAIClient) ExtractLinks(ctx context.Context, html string, baseURL string) ([]string, int, error) {
	if m.extractLinksFn != nil {
		return m.extractLinksFn(ctx, html, baseURL)
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

func TestCrawler_RunWithStructuredData(t *testing.T) {
	// HTTP server that serves pages with JSON-LD product data
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			w.Write([]byte("User-agent: *\nAllow: /\n"))
		case "/":
			w.Write([]byte(`<html><head>
				<script type="application/ld+json">
				[
					{"@type":"Product","name":"Product 1","offers":{"price":"100.00"},"image":"https://img.com/1.jpg"},
					{"@type":"Product","name":"Product 2","offers":{"price":"200.00"},"image":"https://img.com/2.jpg"},
					{"@type":"Product","name":"Product 3","offers":{"price":"300.00"},"image":"https://img.com/3.jpg"},
					{"@type":"Product","name":"Product 4","offers":{"price":"400.00"},"image":"https://img.com/4.jpg"},
					{"@type":"Product","name":"Product 5","offers":{"price":"500.00"},"image":"https://img.com/5.jpg"}
				]
				</script>
				</head><body>
				<a href="/page2">Page 2</a>
				</body></html>`))
		case "/page2":
			w.Write([]byte(`<html><head>
				<script type="application/ld+json">
				[
					{"@type":"Product","name":"Product 6","offers":{"price":"600.00"},"image":"https://img.com/6.jpg"},
					{"@type":"Product","name":"Product 7","offers":{"price":"700.00"},"image":"https://img.com/7.jpg"},
					{"@type":"Product","name":"Product 8","offers":{"price":"800.00"},"image":"https://img.com/8.jpg"},
					{"@type":"Product","name":"Product 9","offers":{"price":"900.00"},"image":"https://img.com/9.jpg"},
					{"@type":"Product","name":"Product 10","offers":{"price":"1000.00"},"image":"https://img.com/10.jpg"}
				]
				</script>
				</head><body>
				<a href="/page3">Page 3</a>
				</body></html>`))
		case "/page3":
			// Generate enough products to hit minProducts=20
			products := ""
			for i := 11; i <= 25; i++ {
				products += fmt.Sprintf(
					`{"@type":"Product","name":"Product %d","offers":{"price":"%d.00"},"image":"https://img.com/%d.jpg"},`,
					i, i*100, i)
			}
			// Remove trailing comma
			products = products[:len(products)-1]
			w.Write([]byte(fmt.Sprintf(`<html><head>
				<script type="application/ld+json">[%s]</script>
				</head><body></body></html>`, products)))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	pool := setupTestDB(t)
	logDir := t.TempDir()

	mockAI := &mockAIClient{
		extractLinksFn: func(ctx context.Context, html string, baseURL string) ([]string, int, error) {
			// AI suggests links if structured data didn't yield enough
			return []string{"/page2", "/page3"}, 100, nil
		},
	}

	c := crawler.New(
		crawler.NewHTTPFetcher(5*time.Second),
		mockAI,
		store.NewShopStore(pool),
		store.NewCrawlStore(pool),
		store.NewProductStore(pool),
		logDir,
	)

	cfg := crawler.CrawlConfig{
		URL:         server.URL,
		Timeout:     30 * time.Second,
		MinProducts: 20,
		Verbose:     false,
	}

	result, err := c.Run(context.Background(), cfg, nil)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, result.ProductsFound, 20)
	assert.Greater(t, result.PagesVisited, 0)
	assert.NotEmpty(t, result.CrawlID)
	assert.NotEmpty(t, result.ShopID)
}

func TestCrawler_RunWithAIFallback(t *testing.T) {
	// HTTP server with NO structured data - requires AI extraction
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			w.WriteHeader(http.StatusNotFound)
		case "/":
			w.Write([]byte(`<html><body>
				<h1>Shop Products</h1>
				<div class="product"><span>Laptop Dell</span><span>5999 PLN</span></div>
				<div class="product"><span>iPhone 15</span><span>4999 PLN</span></div>
				<a href="/page2">More products</a>
				</body></html>`))
		case "/page2":
			w.Write([]byte(`<html><body>
				<div class="product"><span>Samsung Galaxy</span><span>3999 PLN</span></div>
				</body></html>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	pool := setupTestDB(t)
	logDir := t.TempDir()

	callCount := 0
	mockAI := &mockAIClient{
		extractProductsFn: func(ctx context.Context, html string, pageURL string) ([]crawler.RawProduct, int, error) {
			callCount++
			// Generate 12 unique products per page to exceed minProducts
			var products []crawler.RawProduct
			base := (callCount - 1) * 12
			for i := 0; i < 12; i++ {
				products = append(products, crawler.RawProduct{
					Name:     fmt.Sprintf("AI Product %d", base+i+1),
					Price:    float64((base+i+1)*100) + 0.99,
					ImageURL: fmt.Sprintf("https://img.com/ai-%d.jpg", base+i+1),
				})
			}
			return products, 300, nil
		},
		extractLinksFn: func(ctx context.Context, html string, baseURL string) ([]string, int, error) {
			return []string{"/page2"}, 100, nil
		},
	}

	c := crawler.New(
		crawler.NewHTTPFetcher(5*time.Second),
		mockAI,
		store.NewShopStore(pool),
		store.NewCrawlStore(pool),
		store.NewProductStore(pool),
		logDir,
	)

	cfg := crawler.CrawlConfig{
		URL:         server.URL,
		Timeout:     30 * time.Second,
		MinProducts: 20,
		Verbose:     false,
	}

	result, err := c.Run(context.Background(), cfg, nil)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, result.ProductsFound, 20)
	assert.Greater(t, result.AIRequestsCount, 0)
}

func TestCrawler_RunRespectsTimeout(t *testing.T) {
	// HTTP server that responds slowly
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		time.Sleep(100 * time.Millisecond)
		w.Write([]byte(`<html><body>
			<a href="/page2">next</a>
			</body></html>`))
	}))
	defer server.Close()

	pool := setupTestDB(t)
	logDir := t.TempDir()

	mockAI := &mockAIClient{
		extractLinksFn: func(ctx context.Context, html string, baseURL string) ([]string, int, error) {
			return []string{"/page2"}, 100, nil
		},
	}

	c := crawler.New(
		crawler.NewHTTPFetcher(5*time.Second),
		mockAI,
		store.NewShopStore(pool),
		store.NewCrawlStore(pool),
		store.NewProductStore(pool),
		logDir,
	)

	cfg := crawler.CrawlConfig{
		URL:         server.URL,
		Timeout:     500 * time.Millisecond,
		MinProducts: 100, // Impossible to reach
		Verbose:     false,
	}

	result, err := c.Run(context.Background(), cfg, nil)
	// Timeout is not an error - we just return what we have
	require.NoError(t, err)
	// Result should exist even with few/no products (crawl completed with timeout)
	assert.NotEmpty(t, result.CrawlID)
}

func TestCrawler_RunSavesToDatabase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			w.WriteHeader(http.StatusNotFound)
		case "/":
			w.Write([]byte(`<html><head>
				<script type="application/ld+json">
				{"@type":"Product","name":"DB Test Product","offers":{"price":"99.99"},"image":"https://img.com/db.jpg"}
				</script>
				</head><body></body></html>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	pool := setupTestDB(t)
	logDir := t.TempDir()

	mockAI := &mockAIClient{}

	shopStore := store.NewShopStore(pool)
	crawlStore := store.NewCrawlStore(pool)
	productStore := store.NewProductStore(pool)

	c := crawler.New(
		crawler.NewHTTPFetcher(5*time.Second),
		mockAI,
		shopStore,
		crawlStore,
		productStore,
		logDir,
	)

	cfg := crawler.CrawlConfig{
		URL:         server.URL,
		Timeout:     10 * time.Second,
		MinProducts: 1,
		Verbose:     false,
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
