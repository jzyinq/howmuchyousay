package store_test

import (
	"context"
	"testing"

	"github.com/jzy/howmuchyousay/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlayerStore_Create(t *testing.T) {
	pool := setupTestDB(t)
	playerStore := store.NewPlayerStore(pool)
	ctx := context.Background()

	_, session := createTestSession(t, pool, "https://player-test.com")

	player, err := playerStore.Create(ctx, session.ID, "Alice", true)
	require.NoError(t, err)
	assert.NotEmpty(t, player.ID)
	assert.Equal(t, "Alice", player.Nick)
	assert.True(t, player.IsHost)
}

func TestPlayerStore_GetBySessionID(t *testing.T) {
	pool := setupTestDB(t)
	playerStore := store.NewPlayerStore(pool)
	ctx := context.Background()

	_, session := createTestSession(t, pool, "https://player-list-test.com")

	_, err := playerStore.Create(ctx, session.ID, "Alice", true)
	require.NoError(t, err)
	_, err = playerStore.Create(ctx, session.ID, "Bob", false)
	require.NoError(t, err)

	players, err := playerStore.GetBySessionID(ctx, session.ID)
	require.NoError(t, err)
	assert.Len(t, players, 2)
}

func TestPlayerStore_CountBySessionID(t *testing.T) {
	pool := setupTestDB(t)
	playerStore := store.NewPlayerStore(pool)
	ctx := context.Background()

	_, session := createTestSession(t, pool, "https://player-count-test.com")

	_, err := playerStore.Create(ctx, session.ID, "Alice", true)
	require.NoError(t, err)
	_, err = playerStore.Create(ctx, session.ID, "Bob", false)
	require.NoError(t, err)
	_, err = playerStore.Create(ctx, session.ID, "Charlie", false)
	require.NoError(t, err)

	count, err := playerStore.CountBySessionID(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}
