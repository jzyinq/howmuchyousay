package models

import (
	"time"

	"github.com/google/uuid"
)

type Shop struct {
	ID             uuid.UUID `json:"id"`
	URL            string    `json:"url"`
	Name           *string   `json:"name"`
	FirstCrawledAt time.Time `json:"first_crawled_at"`
	LastCrawledAt  time.Time `json:"last_crawled_at"`
}
