package server

import (
	"github.com/google/uuid"

	"github.com/jzy/howmuchyousay/internal/game"
	"github.com/jzy/howmuchyousay/internal/models"
)

// ---- Requests ----

type CreateGameRequest struct {
	Nick      string          `json:"nick"       binding:"required,min=1,max=32"`
	ShopURL   string          `json:"shop_url"   binding:"required,url"`
	GameMode  models.GameMode `json:"game_mode"  binding:"required,oneof=comparison guess"`
	SkipCrawl bool            `json:"skip_crawl"`
}

type SessionIDParams struct {
	SessionID uuid.UUID `uri:"session_id" binding:"required,uuid"`
}

type RoundParams struct {
	SessionID uuid.UUID `uri:"session_id" binding:"required,uuid"`
	Number    int       `uri:"number"     binding:"required,gt=0"`
}

type AnswerRequest struct {
	Answer string `json:"answer" binding:"required"`
}

// ---- Responses ----

type CreateGameResponse struct {
	SessionID uuid.UUID `json:"session_id"`
}

type SessionResponse struct {
	ID           uuid.UUID         `json:"id"`
	Status       models.GameStatus `json:"status"`
	GameMode     models.GameMode   `json:"game_mode"`
	RoundsTotal  int               `json:"rounds_total"`
	CurrentRound int               `json:"current_round"`
	ErrorMessage *string           `json:"error_message,omitempty"`
}

type RoundResponse struct {
	Number   int              `json:"number"`
	Type     models.RoundType `json:"type"`
	ProductA ProductDTO       `json:"product_a"`
	ProductB *ProductDTO      `json:"product_b,omitempty"`
}

// ProductDTO deliberately omits Price — players must not see prices before
// answering. Prices only appear in AnswerResponse.CorrectAnswer.
type ProductDTO struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	ImageURL string    `json:"image_url"`
}

type AnswerResponse struct {
	IsCorrect     bool    `json:"is_correct"`
	Points        int     `json:"points"`
	CorrectAnswer string  `json:"correct_answer"`
	PriceA        float64 `json:"price_a"`
	PriceB        float64 `json:"price_b,omitempty"`
}

type ResultsResponse struct {
	SessionID uuid.UUID          `json:"session_id"`
	Rankings  []game.PlayerScore `json:"rankings"`
}

// productToDTO strips the price field from a product for client-facing payloads.
func productToDTO(p models.Product) ProductDTO {
	return ProductDTO{ID: p.ID, Name: p.Name, ImageURL: p.ImageURL}
}
