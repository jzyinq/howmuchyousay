package store_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jzy/howmuchyousay/internal/models"
	"github.com/jzy/howmuchyousay/internal/store"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable"
	}

	if err := store.RunMigrations(dbURL, "../../migrations"); err != nil {
		t.Fatalf("Failed to run migrations on test database: %v", err)
	}

	pool, err := store.ConnectDB(dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	t.Cleanup(func() {
		cleanupTestDB(t, pool)
		pool.Close()
	})

	return pool
}

func cleanupTestDB(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	// Nullify crawl_id in game_sessions first to break circular FK
	_, _ = pool.Exec(context.Background(), "UPDATE game_sessions SET crawl_id = NULL")
	tables := []string{"answers", "rounds", "players", "products", "crawls", "game_sessions", "shops"}
	for _, table := range tables {
		_, err := pool.Exec(context.Background(), "DELETE FROM "+table)
		if err != nil {
			t.Logf("Warning: failed to clean table %s: %v", table, err)
		}
	}
}

// createTestShop creates a shop for testing purposes.
func createTestShop(t *testing.T, pool *pgxpool.Pool, url string) *models.Shop {
	t.Helper()
	ctx := context.Background()
	shopStore := store.NewShopStore(pool)
	shop, err := shopStore.Create(ctx, url)
	require.NoError(t, err)
	return shop
}

// createTestShopAndCrawl creates a shop and a crawl (no session) for testing.
func createTestShopAndCrawl(t *testing.T, pool *pgxpool.Pool, url string) (*models.Shop, *models.Crawl) {
	t.Helper()
	ctx := context.Background()
	shop := createTestShop(t, pool, url)
	crawlStore := store.NewCrawlStore(pool)
	crawl, err := crawlStore.Create(ctx, shop.ID, nil, "/tmp/test.log")
	require.NoError(t, err)
	return shop, crawl
}

// createTestSession creates a shop and a game session for testing.
func createTestSession(t *testing.T, pool *pgxpool.Pool, shopURL string) (*models.Shop, *models.GameSession) {
	t.Helper()
	ctx := context.Background()
	shop := createTestShop(t, pool, shopURL)
	gameStore := store.NewGameStore(pool)
	session, err := gameStore.Create(ctx, shop.ID, "TestHost", models.GameModeComparison, 10)
	require.NoError(t, err)
	return shop, session
}

// createTestProducts creates a shop, session, crawl, and N products for testing.
func createTestProducts(t *testing.T, pool *pgxpool.Pool, shopURL string, count int) (*models.Shop, *models.GameSession, []models.Product) {
	t.Helper()
	ctx := context.Background()
	shop, session := createTestSession(t, pool, shopURL)
	crawlStore := store.NewCrawlStore(pool)
	crawl, err := crawlStore.Create(ctx, shop.ID, nil, "/tmp/test.log")
	require.NoError(t, err)
	productStore := store.NewProductStore(pool)
	var products []models.Product
	for i := 0; i < count; i++ {
		p, err := productStore.Create(ctx, shop.ID, crawl.ID,
			fmt.Sprintf("Product %d", i), float64((i+1)*100), "", "")
		require.NoError(t, err)
		products = append(products, *p)
	}
	return shop, session, products
}

// createTestRound creates a full setup: shop, session, 2 products, 1 comparison round, 1 player.
func createTestRound(t *testing.T, pool *pgxpool.Pool, shopURL string) (*models.GameSession, *models.Round, *models.Player) {
	t.Helper()
	ctx := context.Background()
	_, session, products := createTestProducts(t, pool, shopURL, 2)
	productBID := products[1].ID
	roundStore := store.NewRoundStore(pool)
	round, err := roundStore.Create(ctx, session.ID, 1, models.RoundTypeComparison, products[0].ID, &productBID, "b", 2)
	require.NoError(t, err)
	playerStore := store.NewPlayerStore(pool)
	player, err := playerStore.Create(ctx, session.ID, "TestPlayer", true)
	require.NoError(t, err)
	return session, round, player
}
