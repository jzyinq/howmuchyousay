package main

import (
	"fmt"
	"log"
	"os"

	"github.com/jzy/howmuchyousay/internal/config"
	"github.com/jzy/howmuchyousay/internal/store"
)

func main() {
	cfg := config.Load()

	if err := store.RunMigrations(cfg.DatabaseURL, "./migrations"); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	pool, err := store.ConnectDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	fmt.Printf("Server ready on port %s\n", cfg.ServerPort)
	os.Exit(0)
}
