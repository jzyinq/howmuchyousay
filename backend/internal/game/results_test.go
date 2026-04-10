package game

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jzy/howmuchyousay/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalcResults(t *testing.T) {
	sessionID := uuid.New()

	player1 := models.Player{ID: uuid.New(), SessionID: sessionID, Nick: "Alice", IsHost: true}
	player2 := models.Player{ID: uuid.New(), SessionID: sessionID, Nick: "Bob", IsHost: false}

	round1 := models.Round{ID: uuid.New(), SessionID: sessionID, RoundNumber: 1}
	round2 := models.Round{ID: uuid.New(), SessionID: sessionID, RoundNumber: 2}
	round3 := models.Round{ID: uuid.New(), SessionID: sessionID, RoundNumber: 3}

	now := time.Now()

	answers := []models.Answer{
		// Round 1: Alice 3pts, Bob 0pts
		{ID: uuid.New(), RoundID: round1.ID, PlayerID: player1.ID, IsCorrect: true, PointsEarned: 3, AnsweredAt: now},
		{ID: uuid.New(), RoundID: round1.ID, PlayerID: player2.ID, IsCorrect: false, PointsEarned: 0, AnsweredAt: now},
		// Round 2: Alice 1pt, Bob 2pts
		{ID: uuid.New(), RoundID: round2.ID, PlayerID: player1.ID, IsCorrect: true, PointsEarned: 1, AnsweredAt: now},
		{ID: uuid.New(), RoundID: round2.ID, PlayerID: player2.ID, IsCorrect: true, PointsEarned: 2, AnsweredAt: now},
		// Round 3: Alice 0pts, Bob 5pts
		{ID: uuid.New(), RoundID: round3.ID, PlayerID: player1.ID, IsCorrect: false, PointsEarned: 0, AnsweredAt: now},
		{ID: uuid.New(), RoundID: round3.ID, PlayerID: player2.ID, IsCorrect: true, PointsEarned: 5, AnsweredAt: now},
	}

	rounds := []models.Round{round1, round2, round3}
	players := []models.Player{player1, player2}

	results := CalcResults(players, answers, rounds)
	require.Len(t, results, 2)

	// Bob wins: 7 points (0+2+5), Alice: 4 points (3+1+0)
	assert.Equal(t, 1, results[0].Rank)
	assert.Equal(t, "Bob", results[0].Nick)
	assert.Equal(t, player2.ID, results[0].PlayerID)
	assert.Equal(t, 7, results[0].TotalPoints)
	assert.Equal(t, 2, results[0].CorrectCount)
	assert.Equal(t, 3, results[0].TotalRounds)
	assert.Equal(t, 5, results[0].BestRoundScore)

	assert.Equal(t, 2, results[1].Rank)
	assert.Equal(t, "Alice", results[1].Nick)
	assert.Equal(t, player1.ID, results[1].PlayerID)
	assert.Equal(t, 4, results[1].TotalPoints)
	assert.Equal(t, 2, results[1].CorrectCount)
	assert.Equal(t, 3, results[1].TotalRounds)
	assert.Equal(t, 3, results[1].BestRoundScore)
}

func TestCalcResultsTiebreaker(t *testing.T) {
	sessionID := uuid.New()

	player1 := models.Player{ID: uuid.New(), SessionID: sessionID, Nick: "Alice"}
	player2 := models.Player{ID: uuid.New(), SessionID: sessionID, Nick: "Bob"}

	round1 := models.Round{ID: uuid.New(), SessionID: sessionID, RoundNumber: 1}

	now := time.Now()

	// Same points — tied players share the same rank
	answers := []models.Answer{
		{ID: uuid.New(), RoundID: round1.ID, PlayerID: player1.ID, IsCorrect: true, PointsEarned: 3, AnsweredAt: now},
		{ID: uuid.New(), RoundID: round1.ID, PlayerID: player2.ID, IsCorrect: true, PointsEarned: 3, AnsweredAt: now},
	}

	results := CalcResults([]models.Player{player1, player2}, answers, []models.Round{round1})
	require.Len(t, results, 2)

	// Both have 3 points — same rank
	assert.Equal(t, 1, results[0].Rank)
	assert.Equal(t, 1, results[1].Rank)
	assert.Equal(t, 3, results[0].TotalPoints)
	assert.Equal(t, 3, results[1].TotalPoints)
}

func TestCalcResultsNoAnswers(t *testing.T) {
	sessionID := uuid.New()
	player1 := models.Player{ID: uuid.New(), SessionID: sessionID, Nick: "Alice"}

	round1 := models.Round{ID: uuid.New(), SessionID: sessionID, RoundNumber: 1}

	results := CalcResults([]models.Player{player1}, nil, []models.Round{round1})
	require.Len(t, results, 1)

	assert.Equal(t, 1, results[0].Rank)
	assert.Equal(t, "Alice", results[0].Nick)
	assert.Equal(t, 0, results[0].TotalPoints)
	assert.Equal(t, 0, results[0].CorrectCount)
	assert.Equal(t, 1, results[0].TotalRounds)
	assert.Equal(t, 0, results[0].BestRoundScore)
}

func TestCalcResultsSinglePlayer(t *testing.T) {
	sessionID := uuid.New()
	player := models.Player{ID: uuid.New(), SessionID: sessionID, Nick: "Solo"}

	round1 := models.Round{ID: uuid.New(), SessionID: sessionID, RoundNumber: 1}
	round2 := models.Round{ID: uuid.New(), SessionID: sessionID, RoundNumber: 2}

	now := time.Now()
	answers := []models.Answer{
		{ID: uuid.New(), RoundID: round1.ID, PlayerID: player.ID, IsCorrect: true, PointsEarned: 5, AnsweredAt: now},
		{ID: uuid.New(), RoundID: round2.ID, PlayerID: player.ID, IsCorrect: true, PointsEarned: 2, AnsweredAt: now},
	}

	results := CalcResults([]models.Player{player}, answers, []models.Round{round1, round2})
	require.Len(t, results, 1)

	assert.Equal(t, 1, results[0].Rank)
	assert.Equal(t, "Solo", results[0].Nick)
	assert.Equal(t, 7, results[0].TotalPoints)
	assert.Equal(t, 2, results[0].CorrectCount)
	assert.Equal(t, 2, results[0].TotalRounds)
	assert.Equal(t, 5, results[0].BestRoundScore)
}

func TestCalcResultsThreePlayersWithTie(t *testing.T) {
	sessionID := uuid.New()

	player1 := models.Player{ID: uuid.New(), SessionID: sessionID, Nick: "Alice"}
	player2 := models.Player{ID: uuid.New(), SessionID: sessionID, Nick: "Bob"}
	player3 := models.Player{ID: uuid.New(), SessionID: sessionID, Nick: "Charlie"}

	round1 := models.Round{ID: uuid.New(), SessionID: sessionID, RoundNumber: 1}

	now := time.Now()

	// Alice: 5pts, Bob: 3pts, Charlie: 3pts — Bob and Charlie tied at rank 2
	answers := []models.Answer{
		{ID: uuid.New(), RoundID: round1.ID, PlayerID: player1.ID, IsCorrect: true, PointsEarned: 5, AnsweredAt: now},
		{ID: uuid.New(), RoundID: round1.ID, PlayerID: player2.ID, IsCorrect: true, PointsEarned: 3, AnsweredAt: now},
		{ID: uuid.New(), RoundID: round1.ID, PlayerID: player3.ID, IsCorrect: true, PointsEarned: 3, AnsweredAt: now},
	}

	results := CalcResults([]models.Player{player1, player2, player3}, answers, []models.Round{round1})
	require.Len(t, results, 3)

	assert.Equal(t, 1, results[0].Rank)
	assert.Equal(t, "Alice", results[0].Nick)
	assert.Equal(t, 2, results[1].Rank)
	assert.Equal(t, 2, results[2].Rank)
}
