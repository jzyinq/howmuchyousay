package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jzy/howmuchyousay/internal/models"
)

type AnswerStore struct {
	pool *pgxpool.Pool
}

func NewAnswerStore(pool *pgxpool.Pool) *AnswerStore {
	return &AnswerStore{pool: pool}
}

func (s *AnswerStore) Create(ctx context.Context, roundID, playerID uuid.UUID, answer string, isCorrect bool, pointsEarned int) (*models.Answer, error) {
	a := &models.Answer{}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO answers (round_id, player_id, answer, is_correct, points_earned)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, round_id, player_id, answer, is_correct, points_earned, answered_at`,
		roundID, playerID, answer, isCorrect, pointsEarned,
	).Scan(&a.ID, &a.RoundID, &a.PlayerID, &a.Answer, &a.IsCorrect, &a.PointsEarned, &a.AnsweredAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (s *AnswerStore) GetByRoundID(ctx context.Context, roundID uuid.UUID) ([]models.Answer, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, round_id, player_id, answer, is_correct, points_earned, answered_at
		 FROM answers WHERE round_id = $1
		 ORDER BY answered_at ASC`,
		roundID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var answers []models.Answer
	for rows.Next() {
		var a models.Answer
		if err := rows.Scan(&a.ID, &a.RoundID, &a.PlayerID, &a.Answer,
			&a.IsCorrect, &a.PointsEarned, &a.AnsweredAt); err != nil {
			return nil, err
		}
		answers = append(answers, a)
	}
	return answers, rows.Err()
}

func (s *AnswerStore) CountByRoundID(ctx context.Context, roundID uuid.UUID) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM answers WHERE round_id = $1`,
		roundID,
	).Scan(&count)
	return count, err
}

func (s *AnswerStore) GetPlayerTotalScore(ctx context.Context, sessionID, playerID uuid.UUID) (int, error) {
	var total int
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(a.points_earned), 0)
		 FROM answers a
		 JOIN rounds r ON r.id = a.round_id
		 WHERE r.session_id = $1 AND a.player_id = $2`,
		sessionID, playerID,
	).Scan(&total)
	return total, err
}
