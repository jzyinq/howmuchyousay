package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jzy/howmuchyousay/internal/models"
)

type CrawlStore struct {
	pool *pgxpool.Pool
}

func NewCrawlStore(pool *pgxpool.Pool) *CrawlStore {
	return &CrawlStore{pool: pool}
}

func (s *CrawlStore) Create(ctx context.Context, shopID uuid.UUID, sessionID *uuid.UUID, logFilePath string) (*models.Crawl, error) {
	crawl := &models.Crawl{}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO crawls (shop_id, session_id, log_file_path)
		 VALUES ($1, $2, $3)
		 RETURNING id, shop_id, session_id, status, products_found, pages_visited,
		           ai_requests_count, error_message, log_file_path, started_at, finished_at, duration_ms`,
		shopID, sessionID, logFilePath,
	).Scan(
		&crawl.ID, &crawl.ShopID, &crawl.SessionID, &crawl.Status,
		&crawl.ProductsFound, &crawl.PagesVisited, &crawl.AIRequestsCount,
		&crawl.ErrorMessage, &crawl.LogFilePath, &crawl.StartedAt,
		&crawl.FinishedAt, &crawl.DurationMs,
	)
	if err != nil {
		return nil, err
	}
	return crawl, nil
}

func (s *CrawlStore) GetByID(ctx context.Context, id uuid.UUID) (*models.Crawl, error) {
	crawl := &models.Crawl{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, shop_id, session_id, status, products_found, pages_visited,
		        ai_requests_count, error_message, log_file_path, started_at, finished_at, duration_ms
		 FROM crawls WHERE id = $1`,
		id,
	).Scan(
		&crawl.ID, &crawl.ShopID, &crawl.SessionID, &crawl.Status,
		&crawl.ProductsFound, &crawl.PagesVisited, &crawl.AIRequestsCount,
		&crawl.ErrorMessage, &crawl.LogFilePath, &crawl.StartedAt,
		&crawl.FinishedAt, &crawl.DurationMs,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return crawl, nil
}

func (s *CrawlStore) UpdateStatus(ctx context.Context, id uuid.UUID, status models.CrawlStatus) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE crawls SET status = $1 WHERE id = $2`,
		status, id,
	)
	return err
}

func (s *CrawlStore) Finish(ctx context.Context, id uuid.UUID, status models.CrawlStatus, productsFound, pagesVisited, aiRequests int, errorMessage *string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE crawls SET
			status = $1,
			products_found = $2,
			pages_visited = $3,
			ai_requests_count = $4,
			error_message = $5,
			finished_at = NOW(),
			duration_ms = EXTRACT(EPOCH FROM (NOW() - started_at))::INT * 1000
		 WHERE id = $6`,
		status, productsFound, pagesVisited, aiRequests, errorMessage, id,
	)
	return err
}
