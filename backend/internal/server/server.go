package server

import (
	"log/slog"
	"math/rand/v2"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jzy/howmuchyousay/internal/config"
	"github.com/jzy/howmuchyousay/internal/store"
)

type Deps struct {
	Pool     *pgxpool.Pool
	Sessions *store.GameStore
	Shops    *store.ShopStore
	Players  *store.PlayerStore
	Products *store.ProductStore
	Rounds   *store.RoundStore
	Answers  *store.AnswerStore
	Crawler  Crawler
	Config   *config.Config
	Logger   *slog.Logger
	// Rng is used for deterministic test scenarios. math/rand/v2 *rand.Rand is
	// NOT safe for concurrent use; handlers that need randomness must either
	// serialize access or derive a per-request local RNG from a seed.
	Rng *rand.Rand
}

type Handler struct {
	Deps
}

func New(d Deps) *Handler { return &Handler{Deps: d} }

func (h *Handler) Routes() *gin.Engine {
	r := gin.New()
	r.Use(h.corsMiddleware(), gin.Logger(), gin.Recovery(), h.errorMiddleware())
	api := r.Group("/api")
	{
		api.POST("/game", h.CreateGame)
		api.GET("/game/:session_id", h.GetSession)
		api.GET("/game/:session_id/round/:number", h.GetRound)
		api.POST("/game/:session_id/round/:number/answer", h.PostAnswer)
		api.GET("/game/:session_id/results", h.GetResults)
	}
	return r
}

func (h *Handler) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

