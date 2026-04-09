package models

import (
	"time"

	"github.com/google/uuid"
)

type CrawlStatus string

const (
	CrawlStatusPending    CrawlStatus = "pending"
	CrawlStatusInProgress CrawlStatus = "in_progress"
	CrawlStatusCompleted  CrawlStatus = "completed"
	CrawlStatusFailed     CrawlStatus = "failed"
)

type Crawl struct {
	ID              uuid.UUID   `json:"id"`
	ShopID          uuid.UUID   `json:"shop_id"`
	SessionID       *uuid.UUID  `json:"session_id"`
	Status          CrawlStatus `json:"status"`
	ProductsFound   int         `json:"products_found"`
	PagesVisited    int         `json:"pages_visited"`
	AIRequestsCount int         `json:"ai_requests_count"`
	ErrorMessage    *string     `json:"error_message"`
	LogFilePath     string      `json:"log_file_path"`
	StartedAt       time.Time   `json:"started_at"`
	FinishedAt      *time.Time  `json:"finished_at"`
	DurationMs      *int        `json:"duration_ms"`
}
