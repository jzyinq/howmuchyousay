package server

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jzy/howmuchyousay/internal/models"
)

func loadCorrectAnswer(t *testing.T, h *Handler, sessionID uuid.UUID, roundNumber int) string {
	t.Helper()
	rows, err := h.Rounds.GetBySessionID(context.Background(), sessionID)
	require.NoError(t, err)
	for _, r := range rows {
		if r.RoundNumber == roundNumber {
			return r.CorrectAnswer
		}
	}
	t.Fatalf("round %d not found", roundNumber)
	return ""
}

func TestPostAnswer_Comparison_CorrectAdvancesRound(t *testing.T) {
	h, _, _ := setupTestHandler(t)
	sid := createSkipCrawlSession(t, h, "https://post-cmp-ok.com", "comparison")
	correct := loadCorrectAnswer(t, h, sid, 1)

	w := postJSON(t, h, "/api/game/"+sid.String()+"/round/1/answer", map[string]any{"answer": correct})
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var body AnswerResponse
	decode(t, w.Body, &body)
	assert.True(t, body.IsCorrect)
	assert.Greater(t, body.Points, 0)
	assert.Equal(t, correct, body.CorrectAnswer)

	session, err := h.Sessions.GetByID(context.Background(), sid)
	require.NoError(t, err)
	assert.Equal(t, 2, session.CurrentRound)
	assert.Equal(t, models.GameStatusInProgress, session.Status)
}

func TestPostAnswer_Comparison_InvalidFormat(t *testing.T) {
	h, _, _ := setupTestHandler(t)
	sid := createSkipCrawlSession(t, h, "https://post-cmp-bad.com", "comparison")

	w := postJSON(t, h, "/api/game/"+sid.String()+"/round/1/answer", map[string]any{"answer": "x"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPostAnswer_Guess_InvalidFormat(t *testing.T) {
	h, _, _ := setupTestHandler(t)
	sid := createSkipCrawlSession(t, h, "https://post-guess-bad.com", "guess")

	w := postJSON(t, h, "/api/game/"+sid.String()+"/round/1/answer", map[string]any{"answer": "abc"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPostAnswer_Guess_Success(t *testing.T) {
	h, _, _ := setupTestHandler(t)
	sid := createSkipCrawlSession(t, h, "https://post-guess-ok.com", "guess")
	correct := loadCorrectAnswer(t, h, sid, 1)

	w := postJSON(t, h, "/api/game/"+sid.String()+"/round/1/answer", map[string]any{"answer": correct})
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var body AnswerResponse
	decode(t, w.Body, &body)
	assert.True(t, body.IsCorrect)
	assert.Equal(t, correct, body.CorrectAnswer)
}

func TestPostAnswer_WrongRound_Rejected(t *testing.T) {
	h, _, _ := setupTestHandler(t)
	sid := createSkipCrawlSession(t, h, "https://post-wronground.com", "comparison")

	w := postJSON(t, h, "/api/game/"+sid.String()+"/round/3/answer", map[string]any{"answer": "a"})
	require.Equal(t, http.StatusConflict, w.Code)
	var body map[string]any
	decode(t, w.Body, &body)
	assert.Equal(t, "not_current_round", body["code"])
}

func TestPostAnswer_Duplicate_Returns409WithExistingResult(t *testing.T) {
	h, _, _ := setupTestHandler(t)
	sid := createSkipCrawlSession(t, h, "https://post-dup.com", "comparison")
	correct := loadCorrectAnswer(t, h, sid, 1)

	w1 := postJSON(t, h, "/api/game/"+sid.String()+"/round/1/answer", map[string]any{"answer": correct})
	require.Equal(t, http.StatusOK, w1.Code)

	// Rewind current_round to 1 to simulate a retry hitting the unique constraint.
	_, err := h.Pool.Exec(context.Background(),
		"UPDATE game_sessions SET current_round = 1 WHERE id = $1", sid)
	require.NoError(t, err)

	w2 := postJSON(t, h, "/api/game/"+sid.String()+"/round/1/answer", map[string]any{"answer": correct})
	require.Equal(t, http.StatusConflict, w2.Code)
	var body map[string]any
	decode(t, w2.Body, &body)
	assert.Equal(t, "already_answered", body["code"])
}

func TestPostAnswer_FinalRoundTransitionsToFinished(t *testing.T) {
	h, _, _ := setupTestHandler(t)
	sid := createSkipCrawlSession(t, h, "https://post-final.com", "comparison")

	for i := 1; i <= 10; i++ {
		correct := loadCorrectAnswer(t, h, sid, i)
		w := postJSON(t, h, "/api/game/"+sid.String()+"/round/"+fmt.Sprint(i)+"/answer", map[string]any{"answer": correct})
		require.Equal(t, http.StatusOK, w.Code, "round %d: %s", i, w.Body.String())
	}

	session, err := h.Sessions.GetByID(context.Background(), sid)
	require.NoError(t, err)
	assert.Equal(t, models.GameStatusFinished, session.Status)
}
