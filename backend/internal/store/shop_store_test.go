package store_test

import (
	"context"
	"testing"

	"github.com/jzy/howmuchyousay/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShopStore_Create(t *testing.T) {
	pool := setupTestDB(t)
	s := store.NewShopStore(pool)
	ctx := context.Background()

	shop, err := s.Create(ctx, "https://mediaexpert.pl")
	require.NoError(t, err)
	assert.NotEmpty(t, shop.ID)
	assert.Equal(t, "https://mediaexpert.pl", shop.URL)
	assert.Nil(t, shop.Name)
}

func TestShopStore_GetByURL(t *testing.T) {
	pool := setupTestDB(t)
	s := store.NewShopStore(pool)
	ctx := context.Background()

	created, err := s.Create(ctx, "https://rossmann.pl")
	require.NoError(t, err)

	found, err := s.GetByURL(ctx, "https://rossmann.pl")
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)
	assert.Equal(t, "https://rossmann.pl", found.URL)
}

func TestShopStore_GetByURL_NotFound(t *testing.T) {
	pool := setupTestDB(t)
	s := store.NewShopStore(pool)
	ctx := context.Background()

	found, err := s.GetByURL(ctx, "https://nonexistent.pl")
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestShopStore_GetByID(t *testing.T) {
	pool := setupTestDB(t)
	s := store.NewShopStore(pool)
	ctx := context.Background()

	created, err := s.Create(ctx, "https://allegro.pl")
	require.NoError(t, err)

	found, err := s.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, created.ID, found.ID)
}

func TestShopStore_UpdateLastCrawled(t *testing.T) {
	pool := setupTestDB(t)
	s := store.NewShopStore(pool)
	ctx := context.Background()

	shop, err := s.Create(ctx, "https://empik.com")
	require.NoError(t, err)

	err = s.UpdateLastCrawled(ctx, shop.ID)
	require.NoError(t, err)

	updated, err := s.GetByID(ctx, shop.ID)
	require.NoError(t, err)
	assert.True(t, updated.LastCrawledAt.After(shop.LastCrawledAt) || updated.LastCrawledAt.Equal(shop.LastCrawledAt))
}
