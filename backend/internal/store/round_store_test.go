package store_test

import (
	"context"
	"testing"

	"github.com/jzy/howmuchyousay/internal/models"
	"github.com/jzy/howmuchyousay/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoundStore_Create(t *testing.T) {
	pool := setupTestDB(t)
	roundStore := store.NewRoundStore(pool)
	ctx := context.Background()

	_, session, products := createTestProducts(t, pool, "https://round-test.com", 2)
	productBID := products[1].ID

	round, err := roundStore.Create(ctx, session.ID, 1, models.RoundTypeComparison, products[0].ID, &productBID, "b", 2)
	require.NoError(t, err)
	assert.NotEmpty(t, round.ID)
	assert.Equal(t, 1, round.RoundNumber)
	assert.Equal(t, models.RoundTypeComparison, round.RoundType)
	assert.Equal(t, "b", round.CorrectAnswer)
	assert.Equal(t, 2, round.DifficultyScore)
}

func TestRoundStore_CreateGuessRound(t *testing.T) {
	pool := setupTestDB(t)
	roundStore := store.NewRoundStore(pool)
	ctx := context.Background()

	_, session, products := createTestProducts(t, pool, "https://guess-round-test.com", 1)

	round, err := roundStore.Create(ctx, session.ID, 1, models.RoundTypeGuess, products[0].ID, nil, "100.00", 3)
	require.NoError(t, err)
	assert.Nil(t, round.ProductBID)
	assert.Equal(t, models.RoundTypeGuess, round.RoundType)
}

func TestRoundStore_GetBySessionID(t *testing.T) {
	pool := setupTestDB(t)
	roundStore := store.NewRoundStore(pool)
	ctx := context.Background()

	_, session, products := createTestProducts(t, pool, "https://round-list-test.com", 4)
	productBID1 := products[1].ID
	productBID2 := products[3].ID

	_, err := roundStore.Create(ctx, session.ID, 1, models.RoundTypeComparison, products[0].ID, &productBID1, "a", 1)
	require.NoError(t, err)
	_, err = roundStore.Create(ctx, session.ID, 2, models.RoundTypeComparison, products[2].ID, &productBID2, "b", 3)
	require.NoError(t, err)

	rounds, err := roundStore.GetBySessionID(ctx, session.ID)
	require.NoError(t, err)
	assert.Len(t, rounds, 2)
	assert.Equal(t, 1, rounds[0].RoundNumber)
	assert.Equal(t, 2, rounds[1].RoundNumber)
}

func TestRoundStore_GetBySessionAndNumber(t *testing.T) {
	pool := setupTestDB(t)
	roundStore := store.NewRoundStore(pool)
	ctx := context.Background()

	_, session, products := createTestProducts(t, pool, "https://round-number-test.com", 2)
	productBID := products[1].ID

	_, err := roundStore.Create(ctx, session.ID, 1, models.RoundTypeComparison, products[0].ID, &productBID, "a", 2)
	require.NoError(t, err)

	found, err := roundStore.GetBySessionAndNumber(ctx, session.ID, 1)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, 1, found.RoundNumber)
}
