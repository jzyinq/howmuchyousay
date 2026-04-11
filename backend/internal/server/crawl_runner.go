package server

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/jzy/howmuchyousay/internal/models"
)

// Crawler is the subset of *crawler.Crawler that the server depends on.
// Defined in the consumer package (Go idiom) so tests can inject a fake.
// The real *crawler.Crawler.Run matches this signature exactly.
type Crawler interface {
	Run(ctx context.Context, cfg crawler.CrawlConfig, sessionID *uuid.UUID) (*crawler.CrawlResult, error)
}

func (h *Handler) runCrawlForSession(sessionID, shopID uuid.UUID, shopURL string, mode models.GameMode) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(h.Config.CrawlTimeout)*time.Second,
	)
	defer cancel()

	cfg := crawler.DefaultCrawlConfig(shopURL)
	cfg.MaxScrapes = h.Config.FirecrawlMaxScrapes
	cfg.Timeout = time.Duration(h.Config.CrawlTimeout) * time.Second

	result, err := h.Crawler.Run(ctx, cfg, &sessionID)
	if err != nil {
		h.markSessionFailed(sessionID, err.Error())
		return
	}

	bg := context.Background()

	if err := h.Sessions.SetCrawlID(bg, sessionID, result.CrawlID); err != nil {
		h.markSessionFailed(sessionID, "set crawl id: "+err.Error())
		return
	}

	products, err := h.Products.GetByShopID(bg, shopID)
	if err != nil {
		h.markSessionFailed(sessionID, "load products: "+err.Error())
		return
	}
	if len(products) < 20 {
		h.markSessionFailed(sessionID, "not enough products after crawl")
		return
	}

	defs, err := h.generateRounds(mode, products, defaultRoundsTotal)
	if err != nil {
		h.markSessionFailed(sessionID, "round generation: "+err.Error())
		return
	}
	if err := persistRounds(bg, h.Pool, sessionID, defs); err != nil {
		h.markSessionFailed(sessionID, "persist rounds: "+err.Error())
		return
	}

	if err := h.Sessions.UpdateStatus(bg, h.Pool, sessionID, models.GameStatusReady); err != nil {
		h.markSessionFailed(sessionID, "mark ready: "+err.Error())
		return
	}
}

func (h *Handler) markSessionFailed(sessionID uuid.UUID, msg string) {
	bg := context.Background()
	if err := h.Sessions.SetErrorMessage(bg, sessionID, msg); err != nil {
		h.Logger.Error("set error message", "err", err, "session_id", sessionID)
	}
	if err := h.Sessions.UpdateStatus(bg, h.Pool, sessionID, models.GameStatusFailed); err != nil {
		h.Logger.Error("mark session failed", "err", err, "session_id", sessionID)
	}
}
