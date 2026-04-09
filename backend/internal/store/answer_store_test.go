package store_test

import (
	"context"
	"testing"

	"github.com/jzy/howmuchyousay/internal/models"
	"github.com/jzy/howmuchyousay/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnswerStore_Create(t *testing.T) {
	pool := setupTestDB(t)
	answerStore := store.NewAnswerStore(pool)
	ctx := context.Background()

	_, round, player := createTestRound(t, pool, "https://answer-test.com")

	answer, err := answerStore.Create(ctx, round.ID, player.ID, "b", true, 2)
	require.NoError(t, err)
	assert.NotEmpty(t, answer.ID)
	assert.Equal(t, "b", answer.Answer)
	assert.True(t, answer.IsCorrect)
	assert.Equal(t, 2, answer.PointsEarned)
}

func TestAnswerStore_GetByRoundID(t *testing.T) {
	pool := setupTestDB(t)
	answerStore := store.NewAnswerStore(pool)
	playerStore := store.NewPlayerStore(pool)
	ctx := context.Background()

	session, round, player1 := createTestRound(t, pool, "https://answer-list-test.com")

	player2, err := playerStore.Create(ctx, session.ID, "Player2", false)
	require.NoError(t, err)

	_, err = answerStore.Create(ctx, round.ID, player1.ID, "a", false, 0)
	require.NoError(t, err)
	_, err = answerStore.Create(ctx, round.ID, player2.ID, "b", true, 2)
	require.NoError(t, err)

	answers, err := answerStore.GetByRoundID(ctx, round.ID)
	require.NoError(t, err)
	assert.Len(t, answers, 2)
}

func TestAnswerStore_CountByRoundID(t *testing.T) {
	pool := setupTestDB(t)
	answerStore := store.NewAnswerStore(pool)
	ctx := context.Background()

	_, round, player := createTestRound(t, pool, "https://answer-count-test.com")

	_, err := answerStore.Create(ctx, round.ID, player.ID, "a", false, 0)
	require.NoError(t, err)

	count, err := answerStore.CountByRoundID(ctx, round.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestAnswerStore_GetPlayerTotalScore(t *testing.T) {
	pool := setupTestDB(t)
	answerStore := store.NewAnswerStore(pool)
	roundStore := store.NewRoundStore(pool)
	ctx := context.Background()

	_, session, products := createTestProducts(t, pool, "https://score-test.com", 4)
	playerStore := store.NewPlayerStore(pool)
	player, err := playerStore.Create(ctx, session.ID, "Scorer", true)
	require.NoError(t, err)

	productBID1 := products[1].ID
	productBID2 := products[3].ID

	round1, err := roundStore.Create(ctx, session.ID, 1, models.RoundTypeComparison, products[0].ID, &productBID1, "b", 2)
	require.NoError(t, err)
	round2, err := roundStore.Create(ctx, session.ID, 2, models.RoundTypeComparison, products[2].ID, &productBID2, "a", 3)
	require.NoError(t, err)

	_, err = answerStore.Create(ctx, round1.ID, player.ID, "b", true, 2)
	require.NoError(t, err)
	_, err = answerStore.Create(ctx, round2.ID, player.ID, "a", true, 3)
	require.NoError(t, err)

	total, err := answerStore.GetPlayerTotalScore(ctx, session.ID, player.ID)
	require.NoError(t, err)
	assert.Equal(t, 5, total)
}
