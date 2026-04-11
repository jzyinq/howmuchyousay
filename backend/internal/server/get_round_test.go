package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jzy/howmuchyousay/internal/models"
)

func createSkipCrawlSession(t *testing.T, h *Handler, shopURL, mode string) uuid.UUID {
	t.Helper()
	_, _ = seedShopWithProducts(t, h.Pool, shopURL, 20)
	w := postJSON(t, h, "/api/game", map[string]any{
		"nick":       "host",
		"shop_url":   shopURL,
		"game_mode":  mode,
		"skip_crawl": true,
	})
	require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())
	var resp CreateGameResponse
	decode(t, w.Body, &resp)
	return resp.SessionID
}

func TestGetRound_NotFoundSession(t *testing.T) {
	h, _, _ := setupTestHandler(t)
	w := getJSON(t, h, "/api/game/"+uuid.New().String()+"/round/1")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetRound_NotCurrentRound(t *testing.T) {
	h, _, _ := setupTestHandler(t)
	sid := createSkipCrawlSession(t, h, "https://getround-notcur.com", "comparison")

	w := getJSON(t, h, "/api/game/"+sid.String()+"/round/3")
	require.Equal(t, http.StatusConflict, w.Code)

	var body map[string]any
	decode(t, w.Body, &body)
	assert.Equal(t, "not_current_round", body["code"])
	assert.Equal(t, float64(1), body["current_round"])
}

func TestGetRound_Success_PriceHidden(t *testing.T) {
	h, _, _ := setupTestHandler(t)
	sid := createSkipCrawlSession(t, h, "https://getround-ok.com", "comparison")

	w := getJSON(t, h, "/api/game/"+sid.String()+"/round/1")
	require.Equal(t, http.StatusOK, w.Code)

	var body RoundResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, 1, body.Number)
	assert.Equal(t, models.RoundTypeComparison, body.Type)
	require.NotNil(t, body.ProductB)

	w2 := getJSON(t, h, "/api/game/"+sid.String()+"/round/1")
	assert.False(t, strings.Contains(strings.ToLower(w2.Body.String()), "price"),
		"round payload leaked a price field: %s", w2.Body.String())
}

func TestGetRound_FinishedSession_Rejected(t *testing.T) {
	h, _, _ := setupTestHandler(t)
	sid := createSkipCrawlSession(t, h, "https://getround-finished.com", "comparison")

	require.NoError(t, h.Sessions.UpdateStatus(context.Background(), h.Pool, sid, models.GameStatusFinished))

	w := getJSON(t, h, "/api/game/"+sid.String()+"/round/1")
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestGetRound_ReadyToInProgressTransition(t *testing.T) {
	h, _, _ := setupTestHandler(t)
	sid := createSkipCrawlSession(t, h, "https://getround-ready.com", "comparison")

	require.NoError(t, h.Sessions.UpdateStatus(context.Background(), h.Pool, sid, models.GameStatusReady))

	w := getJSON(t, h, "/api/game/"+sid.String()+"/round/1")
	require.Equal(t, http.StatusOK, w.Code)

	session, err := h.Sessions.GetByID(context.Background(), sid)
	require.NoError(t, err)
	assert.Equal(t, models.GameStatusInProgress, session.Status)
}

func TestGetRound_InvalidNumber(t *testing.T) {
	h, _, _ := setupTestHandler(t)
	sid := createSkipCrawlSession(t, h, "https://getround-badnum.com", "comparison")

	w := getJSON(t, h, "/api/game/"+sid.String()+"/round/zero")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
