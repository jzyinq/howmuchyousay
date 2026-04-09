package store_test

import (
	"context"
	"testing"

	"github.com/jzy/howmuchyousay/internal/models"
	"github.com/jzy/howmuchyousay/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGameStore_Create(t *testing.T) {
	pool := setupTestDB(t)
	gameStore := store.NewGameStore(pool)
	ctx := context.Background()

	shop := createTestShop(t, pool, "https://game-test.com")

	session, err := gameStore.Create(ctx, shop.ID, "Player1", models.GameModeComparison, 10)
	require.NoError(t, err)
	assert.NotEmpty(t, session.ID)
	assert.Nil(t, session.RoomCode)
	assert.Equal(t, "Player1", session.HostNick)
	assert.Equal(t, models.GameModeComparison, session.GameMode)
	assert.Equal(t, 10, session.RoundsTotal)
	assert.Equal(t, models.GameStatusCrawling, session.Status)
}

func TestGameStore_CreateWithRoomCode(t *testing.T) {
	pool := setupTestDB(t)
	gameStore := store.NewGameStore(pool)
	ctx := context.Background()

	shop := createTestShop(t, pool, "https://room-test.com")
	roomCode := "A3K9F2"

	session, err := gameStore.CreateWithRoom(ctx, shop.ID, "Host1", models.GameModeGuess, 10, roomCode)
	require.NoError(t, err)
	require.NotNil(t, session.RoomCode)
	assert.Equal(t, "A3K9F2", *session.RoomCode)
}

func TestGameStore_GetByID(t *testing.T) {
	pool := setupTestDB(t)
	gameStore := store.NewGameStore(pool)
	ctx := context.Background()

	shop := createTestShop(t, pool, "https://getbyid-test.com")

	created, err := gameStore.Create(ctx, shop.ID, "P1", models.GameModeGuess, 10)
	require.NoError(t, err)

	found, err := gameStore.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, created.ID, found.ID)
}

func TestGameStore_GetByRoomCode(t *testing.T) {
	pool := setupTestDB(t)
	gameStore := store.NewGameStore(pool)
	ctx := context.Background()

	shop := createTestShop(t, pool, "https://roomcode-test.com")

	created, err := gameStore.CreateWithRoom(ctx, shop.ID, "Host", models.GameModeComparison, 10, "XYZ123")
	require.NoError(t, err)

	found, err := gameStore.GetByRoomCode(ctx, "XYZ123")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, created.ID, found.ID)
}

func TestGameStore_UpdateStatus(t *testing.T) {
	pool := setupTestDB(t)
	gameStore := store.NewGameStore(pool)
	ctx := context.Background()

	shop := createTestShop(t, pool, "https://status-test.com")

	session, err := gameStore.Create(ctx, shop.ID, "P1", models.GameModeGuess, 10)
	require.NoError(t, err)

	err = gameStore.UpdateStatus(ctx, session.ID, models.GameStatusInProgress)
	require.NoError(t, err)

	updated, err := gameStore.GetByID(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, models.GameStatusInProgress, updated.Status)
}

func TestGameStore_SetCrawlID(t *testing.T) {
	pool := setupTestDB(t)
	gameStore := store.NewGameStore(pool)
	crawlStore := store.NewCrawlStore(pool)
	ctx := context.Background()

	shop := createTestShop(t, pool, "https://setcrawl-test.com")

	session, err := gameStore.Create(ctx, shop.ID, "P1", models.GameModeGuess, 10)
	require.NoError(t, err)

	crawl, err := crawlStore.Create(ctx, shop.ID, &session.ID, "/tmp/crawl.log")
	require.NoError(t, err)

	err = gameStore.SetCrawlID(ctx, session.ID, crawl.ID)
	require.NoError(t, err)

	updated, err := gameStore.GetByID(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, updated.CrawlID)
	assert.Equal(t, crawl.ID, *updated.CrawlID)
}
