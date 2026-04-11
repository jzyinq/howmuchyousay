package main

import (
	"context"
	"log"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jzy/howmuchyousay/internal/config"
	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/jzy/howmuchyousay/internal/server"
	"github.com/jzy/howmuchyousay/internal/store"
)

func main() {
	cfg := config.Load()

	if err := store.RunMigrations(cfg.DatabaseURL, findMigrationsPath()); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	pool, err := store.ConnectDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	scraper, err := crawler.NewFirecrawlClient(cfg.FirecrawlAPIKey, cfg.FirecrawlAPIURL)
	if err != nil {
		log.Fatalf("firecrawl client: %v", err)
	}
	orch := crawler.NewOrchestrator(cfg.OpenAIAPIKey, cfg.OpenAIModel, "", scraper)
	crawlerImpl := crawler.New(
		scraper,
		orch,
		store.NewShopStore(pool),
		store.NewCrawlStore(pool),
		store.NewProductStore(pool),
		cfg.LogDir,
	)

	h := server.New(server.Deps{
		Pool:     pool,
		Sessions: store.NewGameStore(pool),
		Shops:    store.NewShopStore(pool),
		Players:  store.NewPlayerStore(pool),
		Products: store.NewProductStore(pool),
		Rounds:   store.NewRoundStore(pool),
		Answers:  store.NewAnswerStore(pool),
		Crawler:  crawlerImpl,
		Config:   cfg,
		Logger:   slog.Default(),
		Rng:      rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0)),
	})

	srv := &http.Server{
		Addr:    ":" + cfg.ServerPort,
		Handler: h.Routes(),
	}

	go func() {
		log.Printf("Server listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutdown...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

func findMigrationsPath() string {
	candidates := []string{
		"../migrations",
		"../../migrations",
		"./migrations",
		"backend/migrations",
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
	}
	return "../migrations"
}
