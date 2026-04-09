package models

import (
	"time"

	"github.com/google/uuid"
)

type Answer struct {
	ID           uuid.UUID `json:"id"`
	RoundID      uuid.UUID `json:"round_id"`
	PlayerID     uuid.UUID `json:"player_id"`
	Answer       string    `json:"answer"`
	IsCorrect    bool      `json:"is_correct"`
	PointsEarned int       `json:"points_earned"`
	AnsweredAt   time.Time `json:"answered_at"`
}
