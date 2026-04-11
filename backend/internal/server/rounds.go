package server

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/jzy/howmuchyousay/internal/game"
	"github.com/jzy/howmuchyousay/internal/models"
	"github.com/jzy/howmuchyousay/internal/store"
)

const defaultRoundsTotal = 10

func (h *Handler) generateRounds(mode models.GameMode, products []models.Product, count int) ([]game.RoundDef, error) {
	switch mode {
	case models.GameModeComparison:
		return game.GenerateComparisonRounds(products, count, h.Rng)
	case models.GameModeGuess:
		return game.GenerateGuessRounds(products, count, h.Rng)
	default:
		return nil, fmt.Errorf("unknown game mode: %s", mode)
	}
}

func persistRounds(ctx context.Context, db store.DBExec, sessionID uuid.UUID, defs []game.RoundDef) error {
	rs := store.NewRoundStoreForExec(db)
	for _, d := range defs {
		var productBID *uuid.UUID
		if d.ProductB != nil {
			id := d.ProductB.ID
			productBID = &id
		}
		if _, err := rs.Create(ctx, sessionID, d.RoundNumber, d.RoundType, d.ProductA.ID, productBID, d.CorrectAnswer, d.DifficultyScore); err != nil {
			return fmt.Errorf("persist round %d: %w", d.RoundNumber, err)
		}
	}
	return nil
}
