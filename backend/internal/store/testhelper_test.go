package store_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jzy/howmuchyousay/internal/store"
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

	tables := []string{"answers", "rounds", "players", "products", "crawls", "game_sessions", "shops"}
	for _, table := range tables {
		_, err := pool.Exec(context.Background(), "DELETE FROM "+table)
		if err != nil {
			t.Logf("Warning: failed to clean table %s: %v", table, err)
		}
	}
}
