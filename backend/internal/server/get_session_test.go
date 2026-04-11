package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jzy/howmuchyousay/internal/models"
)

func getJSON(t *testing.T, h *Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)
	return w
}

func TestGetSession_NotFound(t *testing.T) {
	h, _, _ := setupTestHandler(t)
	w := getJSON(t, h, "/api/game/"+uuid.Nil.String())
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetSession_InvalidUUID(t *testing.T) {
	h, _, _ := setupTestHandler(t)
	w := getJSON(t, h, "/api/game/not-a-uuid")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetSession_SkipCrawlCreated_ReturnsInProgress(t *testing.T) {
	h, pool, _ := setupTestHandler(t)
	_, _ = seedShopWithProducts(t, pool, "https://getsession.com", 20)

	w := postJSON(t, h, "/api/game", map[string]any{
		"nick":       "host",
		"shop_url":   "https://getsession.com",
		"game_mode":  "comparison",
		"skip_crawl": true,
	})
	require.Equal(t, http.StatusCreated, w.Code)
	var created CreateGameResponse
	decode(t, w.Body, &created)

	w2 := getJSON(t, h, "/api/game/"+created.SessionID.String())
	require.Equal(t, http.StatusOK, w2.Code)

	var body SessionResponse
	require.NoError(t, json.NewDecoder(w2.Body).Decode(&body))
	assert.Equal(t, created.SessionID, body.ID)
	assert.Equal(t, models.GameStatusInProgress, body.Status)
	assert.Equal(t, 1, body.CurrentRound)
	assert.Equal(t, 10, body.RoundsTotal)
	assert.Nil(t, body.ErrorMessage)
}
