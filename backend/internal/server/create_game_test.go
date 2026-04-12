package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/jzy/howmuchyousay/internal/models"
	"github.com/jzy/howmuchyousay/internal/store"
)

func postJSON(t *testing.T, h *Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf := &bytes.Buffer{}
	if body != nil {
		require.NoError(t, json.NewEncoder(buf).Encode(body))
	}
	req := httptest.NewRequest(http.MethodPost, path, buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)
	return w
}

func decode(t *testing.T, body io.Reader, v any) {
	t.Helper()
	require.NoError(t, json.NewDecoder(body).Decode(v))
}

func TestCreateGame_Validation_BadJSON(t *testing.T) {
	h, _, _ := setupTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/game", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateGame_Validation_MissingFields(t *testing.T) {
	h, _, _ := setupTestHandler(t)
	w := postJSON(t, h, "/api/game", map[string]any{"nick": "me"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateGame_Validation_UnknownGameMode(t *testing.T) {
	h, _, _ := setupTestHandler(t)
	w := postJSON(t, h, "/api/game", map[string]any{
		"nick":      "me",
		"shop_url":  "https://x.com",
		"game_mode": "weirdo",
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateGame_SkipCrawl_Success(t *testing.T) {
	h, pool, _ := setupTestHandler(t)
	ctx := context.Background()
	_, _ = seedShopWithProducts(t, pool, "skipcrawl-success.com", 20)

	w := postJSON(t, h, "/api/game", map[string]any{
		"nick":       "host",
		"shop_url":   "https://skipcrawl-success.com",
		"game_mode":  "comparison",
		"skip_crawl": true,
	})
	require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())

	var resp CreateGameResponse
	decode(t, w.Body, &resp)
	assert.NotEqual(t, uuid.Nil, resp.SessionID)

	session, err := h.Sessions.GetByID(ctx, resp.SessionID)
	require.NoError(t, err)
	require.NotNil(t, session)
	assert.Equal(t, models.GameStatusInProgress, session.Status)
	assert.Equal(t, 1, session.CurrentRound)
	assert.Equal(t, 10, session.RoundsTotal)

	rounds, err := h.Rounds.GetBySessionID(ctx, resp.SessionID)
	require.NoError(t, err)
	assert.Len(t, rounds, 10)

	players, err := h.Players.GetBySessionID(ctx, resp.SessionID)
	require.NoError(t, err)
	require.Len(t, players, 1)
	assert.Equal(t, "host", players[0].Nick)
	assert.True(t, players[0].IsHost)
}

func TestCreateGame_SkipCrawl_NotEnoughProducts(t *testing.T) {
	h, pool, _ := setupTestHandler(t)
	_, _ = seedShopWithProducts(t, pool, "skipcrawl-few.com", 5)

	w := postJSON(t, h, "/api/game", map[string]any{
		"nick":       "host",
		"shop_url":   "https://skipcrawl-few.com",
		"game_mode":  "comparison",
		"skip_crawl": true,
	})
	require.Equal(t, http.StatusConflict, w.Code, "body: %s", w.Body.String())

	var body map[string]any
	decode(t, w.Body, &body)
	assert.Equal(t, "not_enough_products", body["code"])
}

func TestCreateGame_WithCrawl_Success(t *testing.T) {
	h, pool, fake := setupTestHandler(t)
	ctx := context.Background()

	shop, products := seedShopWithProducts(t, pool, "withcrawl-success.com", 20)

	fake.done = make(chan struct{})
	fake.behavior = func(ctx context.Context, cfg crawler.CrawlConfig, sessionID *uuid.UUID) (*crawler.CrawlResult, error) {
		crawl, err := store.NewCrawlStore(pool).Create(ctx, shop.ID, sessionID, "/tmp/fake.log")
		if err != nil {
			return nil, err
		}
		return &crawler.CrawlResult{
			CrawlID:       crawl.ID,
			ShopID:        shop.ID,
			ProductsFound: len(products),
		}, nil
	}

	w := postJSON(t, h, "/api/game", map[string]any{
		"nick":      "host",
		"shop_url":  "https://withcrawl-success.com",
		"game_mode": "comparison",
	})
	require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())

	var resp CreateGameResponse
	decode(t, w.Body, &resp)

	select {
	case <-fake.done:
	case <-time.After(5 * time.Second):
		t.Fatal("crawl goroutine did not complete")
	}
	waitForStatus(t, h, resp.SessionID, models.GameStatusReady, 2*time.Second)

	session, err := h.Sessions.GetByID(ctx, resp.SessionID)
	require.NoError(t, err)
	assert.Equal(t, models.GameStatusReady, session.Status)

	rounds, err := h.Rounds.GetBySessionID(ctx, resp.SessionID)
	require.NoError(t, err)
	assert.Len(t, rounds, 10)
}

func TestCreateGame_WithCrawl_Failure(t *testing.T) {
	h, _, fake := setupTestHandler(t)
	ctx := context.Background()

	fake.done = make(chan struct{})
	fake.behavior = func(ctx context.Context, cfg crawler.CrawlConfig, sessionID *uuid.UUID) (*crawler.CrawlResult, error) {
		return nil, errors.New("firecrawl exploded")
	}

	w := postJSON(t, h, "/api/game", map[string]any{
		"nick":      "host",
		"shop_url":  "https://withcrawl-fail.com",
		"game_mode": "comparison",
	})
	require.Equal(t, http.StatusCreated, w.Code)

	var resp CreateGameResponse
	decode(t, w.Body, &resp)

	select {
	case <-fake.done:
	case <-time.After(5 * time.Second):
		t.Fatal("crawl goroutine did not complete")
	}
	waitForStatus(t, h, resp.SessionID, models.GameStatusFailed, 2*time.Second)

	session, err := h.Sessions.GetByID(ctx, resp.SessionID)
	require.NoError(t, err)
	assert.Equal(t, models.GameStatusFailed, session.Status)
	require.NotNil(t, session.ErrorMessage)
	assert.Contains(t, *session.ErrorMessage, "firecrawl exploded")
}

func waitForStatus(t *testing.T, h *Handler, id uuid.UUID, target models.GameStatus, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		s, err := h.Sessions.GetByID(context.Background(), id)
		require.NoError(t, err)
		if s != nil && s.Status == target {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("session %s did not reach status %s within %s", id, target, timeout)
}
