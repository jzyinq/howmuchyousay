package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jzy/howmuchyousay/internal/models"
)

type RoundStore struct {
	pool *pgxpool.Pool
}

func NewRoundStore(pool *pgxpool.Pool) *RoundStore {
	return &RoundStore{pool: pool}
}

func (s *RoundStore) Create(ctx context.Context, sessionID uuid.UUID, roundNumber int, roundType models.RoundType, productAID uuid.UUID, productBID *uuid.UUID, correctAnswer string, difficultyScore int) (*models.Round, error) {
	round := &models.Round{}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO rounds (session_id, round_number, round_type, product_a_id, product_b_id, correct_answer, difficulty_score)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, session_id, round_number, round_type, product_a_id, product_b_id, correct_answer, difficulty_score`,
		sessionID, roundNumber, roundType, productAID, productBID, correctAnswer, difficultyScore,
	).Scan(&round.ID, &round.SessionID, &round.RoundNumber, &round.RoundType,
		&round.ProductAID, &round.ProductBID, &round.CorrectAnswer, &round.DifficultyScore)
	if err != nil {
		return nil, err
	}
	return round, nil
}

func (s *RoundStore) GetBySessionID(ctx context.Context, sessionID uuid.UUID) ([]models.Round, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, session_id, round_number, round_type, product_a_id, product_b_id, correct_answer, difficulty_score
		 FROM rounds WHERE session_id = $1
		 ORDER BY round_number ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rounds []models.Round
	for rows.Next() {
		var r models.Round
		if err := rows.Scan(&r.ID, &r.SessionID, &r.RoundNumber, &r.RoundType,
			&r.ProductAID, &r.ProductBID, &r.CorrectAnswer, &r.DifficultyScore); err != nil {
			return nil, err
		}
		rounds = append(rounds, r)
	}
	return rounds, rows.Err()
}

func (s *RoundStore) GetBySessionAndNumber(ctx context.Context, sessionID uuid.UUID, roundNumber int) (*models.Round, error) {
	round := &models.Round{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, session_id, round_number, round_type, product_a_id, product_b_id, correct_answer, difficulty_score
		 FROM rounds WHERE session_id = $1 AND round_number = $2`,
		sessionID, roundNumber,
	).Scan(&round.ID, &round.SessionID, &round.RoundNumber, &round.RoundType,
		&round.ProductAID, &round.ProductBID, &round.CorrectAnswer, &round.DifficultyScore)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return round, nil
}
