package game

import (
	"sort"

	"github.com/google/uuid"
	"github.com/jzy/howmuchyousay/internal/models"
)

// PlayerScore holds the aggregated results for a single player in a game session.
type PlayerScore struct {
	PlayerID       uuid.UUID `json:"player_id"`
	Nick           string    `json:"nick"`
	Rank           int       `json:"rank"`
	TotalPoints    int       `json:"total_points"`
	CorrectCount   int       `json:"correct_count"`
	TotalRounds    int       `json:"total_rounds"`
	BestRoundScore int       `json:"best_round_score"`
}

// CalcResults aggregates answers into player scores, sorted by total points descending.
//
// Parameters:
//   - players: all players in the session
//   - answers: all answers across all rounds in the session
//   - rounds: all rounds in the session (used only for total round count)
//
// Returns a slice of PlayerScore sorted by TotalPoints descending.
// Tied players share the same rank (e.g., 1st, 2nd, 2nd, 4th).
func CalcResults(players []models.Player, answers []models.Answer, rounds []models.Round) []PlayerScore {
	totalRounds := len(rounds)

	// Index answers by player ID
	type playerAcc struct {
		totalPoints    int
		correctCount   int
		bestRoundScore int
	}
	accByPlayer := make(map[uuid.UUID]*playerAcc)

	for i := range players {
		accByPlayer[players[i].ID] = &playerAcc{}
	}

	for _, a := range answers {
		acc, ok := accByPlayer[a.PlayerID]
		if !ok {
			continue
		}
		acc.totalPoints += a.PointsEarned
		if a.IsCorrect {
			acc.correctCount++
		}
		if a.PointsEarned > acc.bestRoundScore {
			acc.bestRoundScore = a.PointsEarned
		}
	}

	// Build results preserving player order
	results := make([]PlayerScore, len(players))
	for i, p := range players {
		acc := accByPlayer[p.ID]
		results[i] = PlayerScore{
			PlayerID:       p.ID,
			Nick:           p.Nick,
			TotalPoints:    acc.totalPoints,
			CorrectCount:   acc.correctCount,
			TotalRounds:    totalRounds,
			BestRoundScore: acc.bestRoundScore,
		}
	}

	// Stable sort by total points descending
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].TotalPoints > results[j].TotalPoints
	})

	// Assign ranks — tied players share the same rank
	for i := range results {
		if i == 0 || results[i].TotalPoints < results[i-1].TotalPoints {
			results[i].Rank = i + 1
		} else {
			results[i].Rank = results[i-1].Rank
		}
	}

	return results
}
