package store_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/jzy/howmuchyousay/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProductStore_Create(t *testing.T) {
	pool := setupTestDB(t)
	productStore := store.NewProductStore(pool)
	ctx := context.Background()

	shop, crawl := createTestShopAndCrawl(t, pool, "https://product-test.com")

	product, err := productStore.Create(ctx, shop.ID, crawl.ID, "Laptop Dell XPS 15", 5999.99, "https://img.com/dell.jpg", "https://product-test.com/dell-xps")
	require.NoError(t, err)
	assert.NotEmpty(t, product.ID)
	assert.Equal(t, "Laptop Dell XPS 15", product.Name)
	assert.Equal(t, 5999.99, product.Price)
	assert.Equal(t, "https://img.com/dell.jpg", product.ImageURL)
}

func TestProductStore_GetByShopID(t *testing.T) {
	pool := setupTestDB(t)
	productStore := store.NewProductStore(pool)
	ctx := context.Background()

	shop, crawl := createTestShopAndCrawl(t, pool, "https://list-test.com")

	_, err := productStore.Create(ctx, shop.ID, crawl.ID, "Product A", 100.00, "", "")
	require.NoError(t, err)
	_, err = productStore.Create(ctx, shop.ID, crawl.ID, "Product B", 200.00, "", "")
	require.NoError(t, err)
	_, err = productStore.Create(ctx, shop.ID, crawl.ID, "Product C", 300.00, "", "")
	require.NoError(t, err)

	products, err := productStore.GetByShopID(ctx, shop.ID)
	require.NoError(t, err)
	assert.Len(t, products, 3)
}

func TestProductStore_CountByShopID(t *testing.T) {
	pool := setupTestDB(t)
	productStore := store.NewProductStore(pool)
	ctx := context.Background()

	shop, crawl := createTestShopAndCrawl(t, pool, "https://count-test.com")

	_, err := productStore.Create(ctx, shop.ID, crawl.ID, "Item 1", 50.00, "", "")
	require.NoError(t, err)
	_, err = productStore.Create(ctx, shop.ID, crawl.ID, "Item 2", 75.00, "", "")
	require.NoError(t, err)

	count, err := productStore.CountByShopID(ctx, shop.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestProductStore_GetRandomByShopID(t *testing.T) {
	pool := setupTestDB(t)
	productStore := store.NewProductStore(pool)
	ctx := context.Background()

	shop, crawl := createTestShopAndCrawl(t, pool, "https://random-test.com")

	for i := 0; i < 10; i++ {
		_, err := productStore.Create(ctx, shop.ID, crawl.ID, fmt.Sprintf("Product %d", i), float64(i+1)*10.0, "", "")
		require.NoError(t, err)
	}

	products, err := productStore.GetRandomByShopID(ctx, shop.ID, 5)
	require.NoError(t, err)
	assert.Len(t, products, 5)
}
