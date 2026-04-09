package store_test

import (
	"context"
	"testing"

	"github.com/jzy/howmuchyousay/internal/models"
	"github.com/jzy/howmuchyousay/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCrawlStore_Create(t *testing.T) {
	pool := setupTestDB(t)
	shopStore := store.NewShopStore(pool)
	crawlStore := store.NewCrawlStore(pool)
	ctx := context.Background()

	shop, err := shopStore.Create(ctx, "https://mediaexpert.pl")
	require.NoError(t, err)

	crawl, err := crawlStore.Create(ctx, shop.ID, nil, "/tmp/crawl.log")
	require.NoError(t, err)
	assert.NotEmpty(t, crawl.ID)
	assert.Equal(t, shop.ID, crawl.ShopID)
	assert.Nil(t, crawl.SessionID)
	assert.Equal(t, models.CrawlStatusPending, crawl.Status)
	assert.Equal(t, "/tmp/crawl.log", crawl.LogFilePath)
}

func TestCrawlStore_UpdateStatus(t *testing.T) {
	pool := setupTestDB(t)
	shopStore := store.NewShopStore(pool)
	crawlStore := store.NewCrawlStore(pool)
	ctx := context.Background()

	shop, err := shopStore.Create(ctx, "https://example.com")
	require.NoError(t, err)

	crawl, err := crawlStore.Create(ctx, shop.ID, nil, "/tmp/test.log")
	require.NoError(t, err)

	err = crawlStore.UpdateStatus(ctx, crawl.ID, models.CrawlStatusInProgress)
	require.NoError(t, err)

	updated, err := crawlStore.GetByID(ctx, crawl.ID)
	require.NoError(t, err)
	assert.Equal(t, models.CrawlStatusInProgress, updated.Status)
}

func TestCrawlStore_Finish(t *testing.T) {
	pool := setupTestDB(t)
	shopStore := store.NewShopStore(pool)
	crawlStore := store.NewCrawlStore(pool)
	ctx := context.Background()

	shop, err := shopStore.Create(ctx, "https://finish-test.com")
	require.NoError(t, err)

	crawl, err := crawlStore.Create(ctx, shop.ID, nil, "/tmp/finish.log")
	require.NoError(t, err)

	err = crawlStore.Finish(ctx, crawl.ID, models.CrawlStatusCompleted, 25, 10, 5, nil)
	require.NoError(t, err)

	finished, err := crawlStore.GetByID(ctx, crawl.ID)
	require.NoError(t, err)
	assert.Equal(t, models.CrawlStatusCompleted, finished.Status)
	assert.Equal(t, 25, finished.ProductsFound)
	assert.Equal(t, 10, finished.PagesVisited)
	assert.Equal(t, 5, finished.AIRequestsCount)
	assert.NotNil(t, finished.FinishedAt)
	assert.NotNil(t, finished.DurationMs)
	assert.Nil(t, finished.ErrorMessage)
}

func TestCrawlStore_FinishWithError(t *testing.T) {
	pool := setupTestDB(t)
	shopStore := store.NewShopStore(pool)
	crawlStore := store.NewCrawlStore(pool)
	ctx := context.Background()

	shop, err := shopStore.Create(ctx, "https://error-test.com")
	require.NoError(t, err)

	crawl, err := crawlStore.Create(ctx, shop.ID, nil, "/tmp/error.log")
	require.NoError(t, err)

	errMsg := "timeout exceeded"
	err = crawlStore.Finish(ctx, crawl.ID, models.CrawlStatusFailed, 3, 2, 1, &errMsg)
	require.NoError(t, err)

	finished, err := crawlStore.GetByID(ctx, crawl.ID)
	require.NoError(t, err)
	assert.Equal(t, models.CrawlStatusFailed, finished.Status)
	require.NotNil(t, finished.ErrorMessage)
	assert.Equal(t, "timeout exceeded", *finished.ErrorMessage)
}
