package models

import (
	"github.com/google/uuid"
)

type RoundType string

const (
	RoundTypeComparison RoundType = "comparison"
	RoundTypeGuess      RoundType = "guess"
)

type Round struct {
	ID              uuid.UUID  `json:"id"`
	SessionID       uuid.UUID  `json:"session_id"`
	RoundNumber     int        `json:"round_number"`
	RoundType       RoundType  `json:"round_type"`
	ProductAID      uuid.UUID  `json:"product_a_id"`
	ProductBID      *uuid.UUID `json:"product_b_id"`
	CorrectAnswer   string     `json:"correct_answer"`
	DifficultyScore int        `json:"difficulty_score"`
}
