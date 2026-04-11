package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"os"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/jzy/howmuchyousay/internal/config"
	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/jzy/howmuchyousay/internal/models"
	"github.com/jzy/howmuchyousay/internal/store"
)

// setupTestDB connects to the shared test database, runs migrations, and
// registers cleanup that truncates every game-related table.
func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable"
	}
	require.NoError(t, store.RunMigrations(dbURL, "../../migrations"))

	pool, err := store.ConnectDB(dbURL)
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupTestDB(t, pool)
		pool.Close()
	})
	return pool
}

func cleanupTestDB(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, _ = pool.Exec(context.Background(), "UPDATE game_sessions SET crawl_id = NULL")
	for _, tbl := range []string{"answers", "rounds", "players", "products", "crawls", "game_sessions", "shops"} {
		if _, err := pool.Exec(context.Background(), "DELETE FROM "+tbl); err != nil {
			t.Logf("cleanup: %s: %v", tbl, err)
		}
	}
}

// setupTestHandler returns a Handler wired against the real test DB and a
// controllable fakeCrawler. The returned pool is the same one the Handler holds.
func setupTestHandler(t *testing.T) (*Handler, *pgxpool.Pool, *fakeCrawler) {
	t.Helper()
	pool := setupTestDB(t)
	fake := &fakeCrawler{}
	h := New(Deps{
		Pool:     pool,
		Sessions: store.NewGameStore(pool),
		Shops:    store.NewShopStore(pool),
		Players:  store.NewPlayerStore(pool),
		Products: store.NewProductStore(pool),
		Rounds:   store.NewRoundStore(pool),
		Answers:  store.NewAnswerStore(pool),
		Crawler:  fake,
		Config:   &config.Config{CrawlTimeout: 10, FirecrawlMaxScrapes: 5},
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Rng:      rand.New(rand.NewPCG(42, 0)),
	})
	return h, pool, fake
}

// fakeCrawler lets tests control what Crawler.Run does. If `behavior` is nil,
// Run returns an error. If `done` is non-nil, it is closed once behavior returns
// (for tests that need to wait on the async goroutine without sleeping).
type fakeCrawler struct {
	behavior func(ctx context.Context, cfg crawler.CrawlConfig, sessionID *uuid.UUID) (*crawler.CrawlResult, error)
	done     chan struct{}
	called   atomic.Int32
}

func (f *fakeCrawler) Run(ctx context.Context, cfg crawler.CrawlConfig, sessionID *uuid.UUID) (*crawler.CrawlResult, error) {
	f.called.Add(1)
	defer func() {
		if f.done != nil {
			close(f.done)
		}
	}()
	if f.behavior == nil {
		return nil, fmt.Errorf("fakeCrawler: behavior not set")
	}
	return f.behavior(ctx, cfg, sessionID)
}

// seedShopWithProducts inserts a shop, a crawl row linked to that shop, and N
// products priced at 100, 200, 300, ... so comparison pairs have comfortable
// price deltas. Returns the shop and the created products.
func seedShopWithProducts(t *testing.T, pool *pgxpool.Pool, shopURL string, n int) (*models.Shop, []models.Product) {
	t.Helper()
	ctx := context.Background()

	shop, err := store.NewShopStore(pool).Create(ctx, shopURL)
	require.NoError(t, err)

	crawl, err := store.NewCrawlStore(pool).Create(ctx, shop.ID, nil, "/tmp/test.log")
	require.NoError(t, err)

	productStore := store.NewProductStore(pool)
	products := make([]models.Product, 0, n)
	for i := 0; i < n; i++ {
		p, err := productStore.Create(ctx, shop.ID, crawl.ID,
			fmt.Sprintf("Product %d", i), float64((i+1)*100),
			fmt.Sprintf("https://img/%d.jpg", i), fmt.Sprintf("https://src/%d", i))
		require.NoError(t, err)
		products = append(products, *p)
	}
	return shop, products
}
