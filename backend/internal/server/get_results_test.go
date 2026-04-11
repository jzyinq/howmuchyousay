package server

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jzy/howmuchyousay/internal/models"
)

func TestGetResults_NotFinished(t *testing.T) {
	h, _, _ := setupTestHandler(t)
	sid := createSkipCrawlSession(t, h, "https://results-early.com", "comparison")

	w := getJSON(t, h, "/api/game/"+sid.String()+"/results")
	require.Equal(t, http.StatusConflict, w.Code)
	var body map[string]any
	decode(t, w.Body, &body)
	assert.Equal(t, "game_not_finished", body["code"])
}

func TestGetResults_Success(t *testing.T) {
	h, _, _ := setupTestHandler(t)
	sid := createSkipCrawlSession(t, h, "https://results-ok.com", "comparison")

	for i := 1; i <= 10; i++ {
		correct := loadCorrectAnswer(t, h, sid, i)
		w := postJSON(t, h, "/api/game/"+sid.String()+"/round/"+fmt.Sprint(i)+"/answer", map[string]any{"answer": correct})
		require.Equal(t, http.StatusOK, w.Code, "round %d", i)
	}

	session, err := h.Sessions.GetByID(context.Background(), sid)
	require.NoError(t, err)
	require.Equal(t, models.GameStatusFinished, session.Status)

	w := getJSON(t, h, "/api/game/"+sid.String()+"/results")
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var body ResultsResponse
	decode(t, w.Body, &body)
	assert.Equal(t, sid, body.SessionID)
	require.Len(t, body.Rankings, 1)
	assert.Equal(t, "host", body.Rankings[0].Nick)
	assert.Equal(t, 1, body.Rankings[0].Rank)
	assert.Equal(t, 10, body.Rankings[0].TotalRounds)
}
