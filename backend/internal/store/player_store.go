package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jzy/howmuchyousay/internal/models"
)

type PlayerStore struct {
	pool *pgxpool.Pool
}

func NewPlayerStore(pool *pgxpool.Pool) *PlayerStore {
	return &PlayerStore{pool: pool}
}

func (s *PlayerStore) Create(ctx context.Context, sessionID uuid.UUID, nick string, isHost bool) (*models.Player, error) {
	player := &models.Player{}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO players (session_id, nick, is_host)
		 VALUES ($1, $2, $3)
		 RETURNING id, session_id, nick, joined_at, is_host`,
		sessionID, nick, isHost,
	).Scan(&player.ID, &player.SessionID, &player.Nick, &player.JoinedAt, &player.IsHost)
	if err != nil {
		return nil, err
	}
	return player, nil
}

func (s *PlayerStore) GetBySessionID(ctx context.Context, sessionID uuid.UUID) ([]models.Player, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, session_id, nick, joined_at, is_host
		 FROM players WHERE session_id = $1
		 ORDER BY joined_at ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var players []models.Player
	for rows.Next() {
		var p models.Player
		if err := rows.Scan(&p.ID, &p.SessionID, &p.Nick, &p.JoinedAt, &p.IsHost); err != nil {
			return nil, err
		}
		players = append(players, p)
	}
	return players, rows.Err()
}

func (s *PlayerStore) CountBySessionID(ctx context.Context, sessionID uuid.UUID) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM players WHERE session_id = $1`,
		sessionID,
	).Scan(&count)
	return count, err
}
