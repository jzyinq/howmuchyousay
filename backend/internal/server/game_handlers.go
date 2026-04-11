package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/jzy/howmuchyousay/internal/game"
	"github.com/jzy/howmuchyousay/internal/models"
)

// CreateGame handles POST /api/game.
func (h *Handler) CreateGame(c *gin.Context) {
	var req CreateGameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest(err.Error()))
		return
	}
	ctx := c.Request.Context()

	shop, err := h.Shops.GetByURL(ctx, req.ShopURL)
	if err != nil {
		h.Logger.Error("shops.GetByURL", "err", err)
		c.Error(ErrInternal())
		return
	}
	if shop == nil {
		shop, err = h.Shops.Create(ctx, req.ShopURL)
		if err != nil {
			h.Logger.Error("shops.Create", "err", err)
			c.Error(ErrInternal())
			return
		}
	}

	if req.SkipCrawl {
		h.createGameSkipCrawl(c, req, shop.ID)
		return
	}

	session, err := h.Sessions.Create(ctx, shop.ID, req.Nick, req.GameMode, defaultRoundsTotal)
	if err != nil {
		h.Logger.Error("sessions.Create", "err", err)
		c.Error(ErrInternal())
		return
	}
	if _, err := h.Players.Create(ctx, session.ID, req.Nick, true); err != nil {
		h.Logger.Error("players.Create", "err", err)
		c.Error(ErrInternal())
		return
	}

	go h.runCrawlForSession(session.ID, shop.ID, req.ShopURL, req.GameMode)

	c.JSON(http.StatusCreated, CreateGameResponse{SessionID: session.ID})
}

func (h *Handler) createGameSkipCrawl(c *gin.Context, req CreateGameRequest, shopID uuid.UUID) {
	ctx := c.Request.Context()

	count, err := h.Products.CountByShopID(ctx, shopID)
	if err != nil {
		h.Logger.Error("products.CountByShopID", "err", err)
		c.Error(ErrInternal())
		return
	}
	if count < 20 {
		c.Error(ErrConflict("not_enough_products", "shop has fewer than 20 products").With("count", count))
		return
	}

	products, err := h.Products.GetRandomByShopID(ctx, shopID, 2*defaultRoundsTotal)
	if err != nil {
		h.Logger.Error("products.GetRandomByShopID", "err", err)
		c.Error(ErrInternal())
		return
	}
	defs, err := h.generateRounds(req.GameMode, products, defaultRoundsTotal)
	if err != nil {
		c.Error(ErrConflict("round_generation_failed", err.Error()))
		return
	}

	tx, err := h.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		h.Logger.Error("begin tx", "err", err)
		c.Error(ErrInternal())
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.Background())
		}
	}()

	var sessionID uuid.UUID
	err = tx.QueryRow(ctx,
		`INSERT INTO game_sessions (shop_id, host_nick, game_mode, rounds_total, status)
		 VALUES ($1, $2, $3, $4, 'in_progress')
		 RETURNING id`,
		shopID, req.Nick, req.GameMode, defaultRoundsTotal,
	).Scan(&sessionID)
	if err != nil {
		h.Logger.Error("insert game_sessions", "err", err)
		c.Error(ErrInternal())
		return
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO players (session_id, nick, is_host) VALUES ($1, $2, true)`,
		sessionID, req.Nick,
	)
	if err != nil {
		h.Logger.Error("insert players", "err", err)
		c.Error(ErrInternal())
		return
	}

	if err := persistRounds(ctx, tx, sessionID, defs); err != nil {
		h.Logger.Error("persistRounds", "err", err)
		c.Error(ErrInternal())
		return
	}

	if err := tx.Commit(ctx); err != nil {
		h.Logger.Error("commit", "err", err)
		c.Error(ErrInternal())
		return
	}
	committed = true

	c.JSON(http.StatusCreated, CreateGameResponse{SessionID: sessionID})
}

// GetSession handles GET /api/game/:session_id.
func (h *Handler) GetSession(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("session_id"))
	if err != nil {
		c.Error(ErrBadRequest("invalid session_id"))
		return
	}

	session, err := h.Sessions.GetByID(c.Request.Context(), sessionID)
	if err != nil {
		h.Logger.Error("sessions.GetByID", "err", err)
		c.Error(ErrInternal())
		return
	}
	if session == nil {
		c.Error(ErrNotFound("session"))
		return
	}

	c.JSON(http.StatusOK, SessionResponse{
		ID:           session.ID,
		Status:       session.Status,
		GameMode:     session.GameMode,
		RoundsTotal:  session.RoundsTotal,
		CurrentRound: session.CurrentRound,
		ErrorMessage: session.ErrorMessage,
	})
}

// GetRound handles GET /api/game/:session_id/round/:number.
func (h *Handler) GetRound(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("session_id"))
	if err != nil {
		c.Error(ErrBadRequest("invalid session_id"))
		return
	}
	number, err := strconv.Atoi(c.Param("number"))
	if err != nil || number < 1 {
		c.Error(ErrBadRequest("invalid round number"))
		return
	}
	ctx := c.Request.Context()

	session, err := h.Sessions.GetByID(ctx, sessionID)
	if err != nil {
		h.Logger.Error("sessions.GetByID", "err", err)
		c.Error(ErrInternal())
		return
	}
	if session == nil {
		c.Error(ErrNotFound("session"))
		return
	}

	if session.Status != models.GameStatusReady && session.Status != models.GameStatusInProgress {
		c.Error(ErrConflict("session_not_playable", "session is not playable").
			With("status", string(session.Status)))
		return
	}
	if number != session.CurrentRound {
		c.Error(ErrConflict("not_current_round", "not current round").
			With("current_round", session.CurrentRound))
		return
	}

	round, err := h.Rounds.GetBySessionAndNumber(ctx, sessionID, number)
	if err != nil {
		h.Logger.Error("rounds.GetBySessionAndNumber", "err", err)
		c.Error(ErrInternal())
		return
	}
	if round == nil {
		c.Error(ErrNotFound("round"))
		return
	}

	productA, err := h.Products.GetByID(ctx, round.ProductAID)
	if err != nil || productA == nil {
		h.Logger.Error("load product A", "err", err)
		c.Error(ErrInternal())
		return
	}

	resp := RoundResponse{
		Number:   round.RoundNumber,
		Type:     round.RoundType,
		ProductA: productToDTO(*productA),
	}
	if round.ProductBID != nil {
		productB, err := h.Products.GetByID(ctx, *round.ProductBID)
		if err != nil || productB == nil {
			h.Logger.Error("load product B", "err", err)
			c.Error(ErrInternal())
			return
		}
		dto := productToDTO(*productB)
		resp.ProductB = &dto
	}

	if session.Status == models.GameStatusReady {
		if err := h.Sessions.UpdateStatus(ctx, h.Pool, session.ID, models.GameStatusInProgress); err != nil {
			h.Logger.Error("flip ready→in_progress", "err", err)
			c.Error(ErrInternal())
			return
		}
	}

	c.JSON(http.StatusOK, resp)
}

// PostAnswer handles POST /api/game/:session_id/round/:number/answer.
func (h *Handler) PostAnswer(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("session_id"))
	if err != nil {
		c.Error(ErrBadRequest("invalid session_id"))
		return
	}
	number, err := strconv.Atoi(c.Param("number"))
	if err != nil || number < 1 {
		c.Error(ErrBadRequest("invalid round number"))
		return
	}
	var req AnswerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest(err.Error()))
		return
	}
	ctx := c.Request.Context()

	session, err := h.Sessions.GetByID(ctx, sessionID)
	if err != nil {
		h.Logger.Error("sessions.GetByID", "err", err)
		c.Error(ErrInternal())
		return
	}
	if session == nil {
		c.Error(ErrNotFound("session"))
		return
	}
	if session.Status != models.GameStatusInProgress {
		c.Error(ErrConflict("session_not_in_progress", "session is not in progress").
			With("status", string(session.Status)))
		return
	}
	if number != session.CurrentRound {
		c.Error(ErrConflict("not_current_round", "not current round").
			With("current_round", session.CurrentRound))
		return
	}

	round, err := h.Rounds.GetBySessionAndNumber(ctx, session.ID, number)
	if err != nil {
		h.Logger.Error("rounds.GetBySessionAndNumber", "err", err)
		c.Error(ErrInternal())
		return
	}
	if round == nil {
		c.Error(ErrNotFound("round"))
		return
	}

	productA, err := h.Products.GetByID(ctx, round.ProductAID)
	if err != nil || productA == nil {
		h.Logger.Error("load product A", "err", err)
		c.Error(ErrInternal())
		return
	}
	var productB *models.Product
	if round.ProductBID != nil {
		productB, err = h.Products.GetByID(ctx, *round.ProductBID)
		if err != nil || productB == nil {
			h.Logger.Error("load product B", "err", err)
			c.Error(ErrInternal())
			return
		}
	}

	players, err := h.Players.GetBySessionID(ctx, session.ID)
	if err != nil || len(players) == 0 {
		h.Logger.Error("players.GetBySessionID", "err", err)
		c.Error(ErrInternal())
		return
	}
	playerID := players[0].ID

	var isCorrect bool
	var points int
	var correctAnswerForResponse string

	switch round.RoundType {
	case models.RoundTypeComparison:
		if req.Answer != "a" && req.Answer != "b" {
			c.Error(ErrBadRequest("comparison answer must be 'a' or 'b'"))
			return
		}
		if productB == nil {
			c.Error(ErrInternal())
			return
		}
		isCorrect, points = game.EvalComparison(req.Answer, round.CorrectAnswer, productA.Price, productB.Price)
		correctAnswerForResponse = round.CorrectAnswer

	case models.RoundTypeGuess:
		guessed, parseErr := strconv.ParseFloat(req.Answer, 64)
		if parseErr != nil || guessed <= 0 {
			c.Error(ErrBadRequest("guess answer must be a positive number"))
			return
		}
		isCorrect, points = game.EvalGuess(guessed, productA.Price)
		correctAnswerForResponse = game.FormatCorrectGuessAnswer(productA.Price)

	default:
		c.Error(ErrInternal())
		return
	}

	tx, err := h.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		h.Logger.Error("begin tx", "err", err)
		c.Error(ErrInternal())
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.Background())
		}
	}()

	if _, err := h.Answers.Create(ctx, tx, round.ID, playerID, req.Answer, isCorrect, points); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			_ = tx.Rollback(context.Background())
			existing, getErr := h.Answers.GetByRoundID(context.Background(), round.ID)
			if getErr != nil || len(existing) == 0 {
				h.Logger.Error("duplicate answer but GetByRoundID empty", "err", getErr)
				c.Error(ErrInternal())
				return
			}
			for _, a := range existing {
				if a.PlayerID == playerID {
					c.Error(ErrConflict("already_answered", "answer already submitted").
						With("is_correct", a.IsCorrect).
						With("points", a.PointsEarned).
						With("correct_answer", correctAnswerForResponse))
					return
				}
			}
			c.Error(ErrInternal())
			return
		}
		h.Logger.Error("answers.Create", "err", err)
		c.Error(ErrInternal())
		return
	}

	if err := h.Sessions.IncrementCurrentRound(ctx, tx, session.ID); err != nil {
		h.Logger.Error("increment current_round", "err", err)
		c.Error(ErrInternal())
		return
	}

	if round.RoundNumber == session.RoundsTotal {
		if err := h.Sessions.UpdateStatus(ctx, tx, session.ID, models.GameStatusFinished); err != nil {
			h.Logger.Error("transition to finished", "err", err)
			c.Error(ErrInternal())
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		h.Logger.Error("commit answer tx", "err", err)
		c.Error(ErrInternal())
		return
	}
	committed = true

	c.JSON(http.StatusOK, AnswerResponse{
		IsCorrect:     isCorrect,
		Points:        points,
		CorrectAnswer: correctAnswerForResponse,
	})
}

// GetResults handles GET /api/game/:session_id/results.
func (h *Handler) GetResults(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("session_id"))
	if err != nil {
		c.Error(ErrBadRequest("invalid session_id"))
		return
	}
	ctx := c.Request.Context()

	session, err := h.Sessions.GetByID(ctx, sessionID)
	if err != nil {
		h.Logger.Error("sessions.GetByID", "err", err)
		c.Error(ErrInternal())
		return
	}
	if session == nil {
		c.Error(ErrNotFound("session"))
		return
	}
	if session.Status != models.GameStatusFinished {
		c.Error(ErrConflict("game_not_finished", "game is not finished").
			With("status", string(session.Status)))
		return
	}

	players, err := h.Players.GetBySessionID(ctx, session.ID)
	if err != nil {
		h.Logger.Error("players.GetBySessionID", "err", err)
		c.Error(ErrInternal())
		return
	}
	rounds, err := h.Rounds.GetBySessionID(ctx, session.ID)
	if err != nil {
		h.Logger.Error("rounds.GetBySessionID", "err", err)
		c.Error(ErrInternal())
		return
	}

	var answers []models.Answer
	for _, r := range rounds {
		roundAnswers, err := h.Answers.GetByRoundID(ctx, r.ID)
		if err != nil {
			h.Logger.Error("answers.GetByRoundID", "err", err)
			c.Error(ErrInternal())
			return
		}
		answers = append(answers, roundAnswers...)
	}

	rankings := game.CalcResults(players, answers, rounds)
	c.JSON(http.StatusOK, ResultsResponse{
		SessionID: session.ID,
		Rankings:  rankings,
	})
}
