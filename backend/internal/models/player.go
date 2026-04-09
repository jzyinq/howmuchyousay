package models

import (
	"time"

	"github.com/google/uuid"
)

type Player struct {
	ID        uuid.UUID `json:"id"`
	SessionID uuid.UUID `json:"session_id"`
	Nick      string    `json:"nick"`
	JoinedAt  time.Time `json:"joined_at"`
	IsHost    bool      `json:"is_host"`
}
