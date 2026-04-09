package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jzy/howmuchyousay/internal/models"
)

type ProductStore struct {
	pool *pgxpool.Pool
}

func NewProductStore(pool *pgxpool.Pool) *ProductStore {
	return &ProductStore{pool: pool}
}

func (s *ProductStore) Create(ctx context.Context, shopID, crawlID uuid.UUID, name string, price float64, imageURL, sourceURL string) (*models.Product, error) {
	product := &models.Product{}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO products (shop_id, crawl_id, name, price, image_url, source_url)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, shop_id, crawl_id, name, price, image_url, source_url, created_at`,
		shopID, crawlID, name, price, imageURL, sourceURL,
	).Scan(&product.ID, &product.ShopID, &product.CrawlID, &product.Name,
		&product.Price, &product.ImageURL, &product.SourceURL, &product.CreatedAt)
	if err != nil {
		return nil, err
	}
	return product, nil
}

func (s *ProductStore) GetByShopID(ctx context.Context, shopID uuid.UUID) ([]models.Product, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, shop_id, crawl_id, name, price, image_url, source_url, created_at
		 FROM products WHERE shop_id = $1
		 ORDER BY created_at DESC`,
		shopID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []models.Product
	for rows.Next() {
		var p models.Product
		if err := rows.Scan(&p.ID, &p.ShopID, &p.CrawlID, &p.Name, &p.Price,
			&p.ImageURL, &p.SourceURL, &p.CreatedAt); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, rows.Err()
}

func (s *ProductStore) CountByShopID(ctx context.Context, shopID uuid.UUID) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM products WHERE shop_id = $1`,
		shopID,
	).Scan(&count)
	return count, err
}

func (s *ProductStore) GetRandomByShopID(ctx context.Context, shopID uuid.UUID, limit int) ([]models.Product, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, shop_id, crawl_id, name, price, image_url, source_url, created_at
		 FROM products WHERE shop_id = $1
		 ORDER BY RANDOM()
		 LIMIT $2`,
		shopID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []models.Product
	for rows.Next() {
		var p models.Product
		if err := rows.Scan(&p.ID, &p.ShopID, &p.CrawlID, &p.Name, &p.Price,
			&p.ImageURL, &p.SourceURL, &p.CreatedAt); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, rows.Err()
}
