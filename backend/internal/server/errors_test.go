package server

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestHandlerForMiddleware() *Handler {
	gin.SetMode(gin.TestMode)
	return &Handler{Deps: Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}}
}

func TestErrorMiddleware_APIError_Conflict(t *testing.T) {
	h := newTestHandlerForMiddleware()
	r := gin.New()
	r.Use(h.errorMiddleware())
	r.GET("/boom", func(c *gin.Context) {
		c.Error(ErrConflict("not_current_round", "not current round").With("current_round", 3))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "not current round", body["error"])
	assert.Equal(t, "not_current_round", body["code"])
	assert.Equal(t, float64(3), body["current_round"])
}

func TestErrorMiddleware_UnknownError_500(t *testing.T) {
	h := newTestHandlerForMiddleware()
	r := gin.New()
	r.Use(h.errorMiddleware())
	r.GET("/boom", func(c *gin.Context) {
		c.Error(errors.New("raw non-APIError failure"))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "internal", body["code"])
}

func TestErrorMiddleware_NoError_NoOp(t *testing.T) {
	h := newTestHandlerForMiddleware()
	r := gin.New()
	r.Use(h.errorMiddleware())
	r.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
