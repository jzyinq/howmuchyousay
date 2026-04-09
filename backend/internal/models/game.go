package models

import (
	"time"

	"github.com/google/uuid"
)

type GameMode string

const (
	GameModeComparison GameMode = "comparison"
	GameModeGuess      GameMode = "guess"
)

type GameStatus string

const (
	GameStatusCrawling   GameStatus = "crawling"
	GameStatusReady      GameStatus = "ready"
	GameStatusLobby      GameStatus = "lobby"
	GameStatusInProgress GameStatus = "in_progress"
	GameStatusFinished   GameStatus = "finished"
)

type GameSession struct {
	ID          uuid.UUID  `json:"id"`
	RoomCode    *string    `json:"room_code"`
	HostNick    string     `json:"host_nick"`
	ShopID      uuid.UUID  `json:"shop_id"`
	GameMode    GameMode   `json:"game_mode"`
	RoundsTotal int        `json:"rounds_total"`
	Status      GameStatus `json:"status"`
	CrawlID     *uuid.UUID `json:"crawl_id"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
