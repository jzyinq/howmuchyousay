package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jzy/howmuchyousay/internal/models"
)

type ShopStore struct {
	pool *pgxpool.Pool
}

func NewShopStore(pool *pgxpool.Pool) *ShopStore {
	return &ShopStore{pool: pool}
}

func (s *ShopStore) Create(ctx context.Context, url string) (*models.Shop, error) {
	shop := &models.Shop{}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO shops (url) VALUES ($1)
		 RETURNING id, url, name, first_crawled_at, last_crawled_at`,
		url,
	).Scan(&shop.ID, &shop.URL, &shop.Name, &shop.FirstCrawledAt, &shop.LastCrawledAt)
	if err != nil {
		return nil, err
	}
	return shop, nil
}

func (s *ShopStore) GetByURL(ctx context.Context, url string) (*models.Shop, error) {
	shop := &models.Shop{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, url, name, first_crawled_at, last_crawled_at FROM shops WHERE url = $1`,
		url,
	).Scan(&shop.ID, &shop.URL, &shop.Name, &shop.FirstCrawledAt, &shop.LastCrawledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return shop, nil
}

func (s *ShopStore) GetByID(ctx context.Context, id uuid.UUID) (*models.Shop, error) {
	shop := &models.Shop{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, url, name, first_crawled_at, last_crawled_at FROM shops WHERE id = $1`,
		id,
	).Scan(&shop.ID, &shop.URL, &shop.Name, &shop.FirstCrawledAt, &shop.LastCrawledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return shop, nil
}

func (s *ShopStore) UpdateLastCrawled(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE shops SET last_crawled_at = NOW() WHERE id = $1`,
		id,
	)
	return err
}
