package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jzy/howmuchyousay/internal/models"
)

type GameStore struct {
	pool *pgxpool.Pool
}

func NewGameStore(pool *pgxpool.Pool) *GameStore {
	return &GameStore{pool: pool}
}

func (s *GameStore) Create(ctx context.Context, shopID uuid.UUID, hostNick string, mode models.GameMode, roundsTotal int) (*models.GameSession, error) {
	session := &models.GameSession{}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO game_sessions (shop_id, host_nick, game_mode, rounds_total)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, room_code, host_nick, shop_id, game_mode, rounds_total, status, crawl_id, current_round, error_message, created_at, updated_at`,
		shopID, hostNick, mode, roundsTotal,
	).Scan(&session.ID, &session.RoomCode, &session.HostNick, &session.ShopID,
		&session.GameMode, &session.RoundsTotal, &session.Status, &session.CrawlID,
		&session.CurrentRound, &session.ErrorMessage,
		&session.CreatedAt, &session.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (s *GameStore) CreateWithRoom(ctx context.Context, shopID uuid.UUID, hostNick string, mode models.GameMode, roundsTotal int, roomCode string) (*models.GameSession, error) {
	session := &models.GameSession{}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO game_sessions (shop_id, host_nick, game_mode, rounds_total, room_code, status)
		 VALUES ($1, $2, $3, $4, $5, 'lobby')
		 RETURNING id, room_code, host_nick, shop_id, game_mode, rounds_total, status, crawl_id, current_round, error_message, created_at, updated_at`,
		shopID, hostNick, mode, roundsTotal, roomCode,
	).Scan(&session.ID, &session.RoomCode, &session.HostNick, &session.ShopID,
		&session.GameMode, &session.RoundsTotal, &session.Status, &session.CrawlID,
		&session.CurrentRound, &session.ErrorMessage,
		&session.CreatedAt, &session.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (s *GameStore) GetByID(ctx context.Context, id uuid.UUID) (*models.GameSession, error) {
	session := &models.GameSession{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, room_code, host_nick, shop_id, game_mode, rounds_total, status, crawl_id, current_round, error_message, created_at, updated_at
		 FROM game_sessions WHERE id = $1`,
		id,
	).Scan(&session.ID, &session.RoomCode, &session.HostNick, &session.ShopID,
		&session.GameMode, &session.RoundsTotal, &session.Status, &session.CrawlID,
		&session.CurrentRound, &session.ErrorMessage,
		&session.CreatedAt, &session.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (s *GameStore) GetByRoomCode(ctx context.Context, code string) (*models.GameSession, error) {
	session := &models.GameSession{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, room_code, host_nick, shop_id, game_mode, rounds_total, status, crawl_id, current_round, error_message, created_at, updated_at
		 FROM game_sessions WHERE room_code = $1`,
		code,
	).Scan(&session.ID, &session.RoomCode, &session.HostNick, &session.ShopID,
		&session.GameMode, &session.RoundsTotal, &session.Status, &session.CrawlID,
		&session.CurrentRound, &session.ErrorMessage,
		&session.CreatedAt, &session.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (s *GameStore) UpdateStatus(ctx context.Context, db DBExec, id uuid.UUID, status models.GameStatus) error {
	_, err := db.Exec(ctx,
		`UPDATE game_sessions SET status = $1, updated_at = NOW() WHERE id = $2`,
		status, id,
	)
	return err
}

func (s *GameStore) SetCrawlID(ctx context.Context, sessionID, crawlID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE game_sessions SET crawl_id = $1, updated_at = NOW() WHERE id = $2`,
		crawlID, sessionID,
	)
	return err
}

// IncrementCurrentRound atomically bumps current_round by 1. Takes a DBExec
// so it can run inside the answer handler's transaction alongside the answer
// insert and the (last-round) status update.
func (s *GameStore) IncrementCurrentRound(ctx context.Context, db DBExec, id uuid.UUID) error {
	_, err := db.Exec(ctx,
		`UPDATE game_sessions SET current_round = current_round + 1, updated_at = NOW() WHERE id = $1`,
		id,
	)
	return err
}

// SetErrorMessage writes error_message. Used by the crawl goroutine's
// markSessionFailed helper; not transactional (a best-effort single-row update).
func (s *GameStore) SetErrorMessage(ctx context.Context, id uuid.UUID, msg string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE game_sessions SET error_message = $1, updated_at = NOW() WHERE id = $2`,
		msg, id,
	)
	return err
}
