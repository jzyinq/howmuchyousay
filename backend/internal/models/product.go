package models

import (
	"time"

	"github.com/google/uuid"
)

type Product struct {
	ID        uuid.UUID `json:"id"`
	ShopID    uuid.UUID `json:"shop_id"`
	CrawlID   uuid.UUID `json:"crawl_id"`
	Name      string    `json:"name"`
	Price     float64   `json:"price"`
	ImageURL  string    `json:"image_url"`
	SourceURL string    `json:"source_url"`
	CreatedAt time.Time `json:"created_at"`
}
