# Plan 1: Database, Models & Store Layer

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Set up the Go backend project skeleton, PostgreSQL schema, Go models, and store (repository) layer with full test coverage.

**Architecture:** Standard Go project layout with `cmd/` for entry points, `internal/` for private packages. Store layer uses `database/sql` with `pgx` driver directly (no ORM). Migrations managed by `golang-migrate`. Tests run against a real PostgreSQL instance via docker-compose.

**Tech Stack:** Go 1.23+, PostgreSQL 16, pgx v5, golang-migrate, testify, docker-compose

**Plan sequence:** This is Plan 1 of 5:
1. **Database, Models & Store Layer** (this plan)
2. Crawler CLI & AI Agent
3. Game Engine
4. Single Player (API + Frontend)
5. Multiplayer (WebSocket + Frontend)

---

## File Structure

```
backend/
├── cmd/
│   └── server/
│       └── main.go              # Minimal entry point (just boots config + DB, no routes yet)
├── internal/
│   ├── config/
│   │   └── config.go            # Env-based configuration (DB_URL, OPENAI_API_KEY, etc.)
│   ├── models/
│   │   ├── shop.go              # Shop struct
│   │   ├── crawl.go             # Crawl struct + status enum
│   │   ├── product.go           # Product struct
│   │   ├── game.go              # GameSession struct + status/mode enums
│   │   ├── player.go            # Player struct
│   │   ├── round.go             # Round struct + type enum
│   │   └── answer.go            # Answer struct
│   └── store/
│       ├── db.go                # DB connection helper (open, ping, close)
│       ├── shop_store.go        # CRUD for shops
│       ├── shop_store_test.go
│       ├── crawl_store.go       # CRUD for crawls
│       ├── crawl_store_test.go
│       ├── product_store.go     # CRUD for products
│       ├── product_store_test.go
│       ├── game_store.go        # CRUD for game_sessions
│       ├── game_store_test.go
│       ├── player_store.go      # CRUD for players
│       ├── player_store_test.go
│       ├── round_store.go       # CRUD for rounds
│       ├── round_store_test.go
│       ├── answer_store.go      # CRUD for answers
│       ├── answer_store_test.go
│       └── testhelper_test.go   # Shared test setup (DB connection, cleanup)
├── migrations/
│   ├── 001_create_shops.up.sql
│   ├── 001_create_shops.down.sql
│   ├── 002_create_game_sessions.up.sql
│   ├── 002_create_game_sessions.down.sql
│   ├── 003_create_crawls.up.sql
│   ├── 003_create_crawls.down.sql
│   ├── 004_create_products.up.sql
│   ├── 004_create_products.down.sql
│   ├── 005_create_players.up.sql
│   ├── 005_create_players.down.sql
│   ├── 006_create_rounds.up.sql
│   ├── 006_create_rounds.down.sql
│   ├── 007_create_answers.up.sql
│   └── 007_create_answers.down.sql
├── go.mod
└── go.sum
docker-compose.yml               # PostgreSQL for dev + test
.env.example
Makefile
.gitignore
```

Note on migration order: `game_sessions` is created before `crawls` because `crawls.session_id` references `game_sessions`. `shops` has no FK dependencies so it goes first.

---

### Task 1: Project Skeleton & Dependencies

**Files:**
- Create: `backend/go.mod`
- Create: `backend/cmd/server/main.go`
- Create: `docker-compose.yml`
- Create: `.env.example`
- Create: `Makefile`
- Create: `.gitignore`

- [ ] **Step 1: Initialize Go module**

```bash
cd backend && go mod init github.com/jzy/howmuchyousay
```

- [ ] **Step 2: Create minimal main.go**

Create `backend/cmd/server/main.go`:

```go
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("howmuchyousay server")
	os.Exit(0)
}
```

- [ ] **Step 3: Verify it compiles**

Run: `cd backend && go build ./cmd/server/`
Expected: No errors, binary created.

- [ ] **Step 4: Create docker-compose.yml**

Create `docker-compose.yml` at project root:

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: hmys
      POSTGRES_PASSWORD: hmys_dev
      POSTGRES_DB: howmuchyousay
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data

  postgres-test:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: hmys
      POSTGRES_PASSWORD: hmys_test
      POSTGRES_DB: howmuchyousay_test
    ports:
      - "5433:5432"

volumes:
  pgdata:
```

- [ ] **Step 5: Create .env.example**

Create `.env.example`:

```
DATABASE_URL=postgres://hmys:hmys_dev@localhost:5432/howmuchyousay?sslmode=disable
TEST_DATABASE_URL=postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable
OPENAI_API_KEY=sk-your-key-here
LOG_DIR=./logs
```

- [ ] **Step 6: Create Makefile**

Create `Makefile`:

```makefile
.PHONY: dev test migrate-up migrate-down build

dev:
	docker compose up -d postgres
	cd backend && go run ./cmd/server/

test:
	docker compose up -d postgres-test
	cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./... -v

migrate-up:
	cd backend && go run -tags migrate ./cmd/server/ -migrate up

migrate-down:
	cd backend && go run -tags migrate ./cmd/server/ -migrate down

build:
	cd backend && go build -o ../bin/server ./cmd/server/
	cd backend && go build -o ../bin/crawler ./cmd/crawler/
```

- [ ] **Step 7: Create .gitignore**

Create `.gitignore`:

```
# Binaries
bin/
backend/server
backend/crawler

# Environment
.env

# Logs
backend/logs/

# IDE
.idea/
.vscode/
*.swp

# OS
.DS_Store

# Go
vendor/

# Docker volumes
pgdata/
```

- [ ] **Step 8: Install Go dependencies**

```bash
cd backend && go get github.com/jackc/pgx/v5
cd backend && go get github.com/jackc/pgx/v5/stdlib
cd backend && go get github.com/golang-migrate/migrate/v4
cd backend && go get github.com/golang-migrate/migrate/v4/database/postgres
cd backend && go get github.com/golang-migrate/migrate/v4/source/file
cd backend && go get github.com/google/uuid
cd backend && go get github.com/stretchr/testify
```

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "feat: project skeleton with Go module, docker-compose, Makefile"
```

---

### Task 2: Configuration

**Files:**
- Create: `backend/internal/config/config.go`

- [ ] **Step 1: Write config.go**

Create `backend/internal/config/config.go`:

```go
package config

import (
	"os"
)

type Config struct {
	DatabaseURL  string
	OpenAIAPIKey string
	LogDir       string
	ServerPort   string
}

func Load() *Config {
	return &Config{
		DatabaseURL:  getEnv("DATABASE_URL", "postgres://hmys:hmys_dev@localhost:5432/howmuchyousay?sslmode=disable"),
		OpenAIAPIKey: getEnv("OPENAI_API_KEY", ""),
		LogDir:       getEnv("LOG_DIR", "./logs"),
		ServerPort:   getEnv("SERVER_PORT", "8080"),
	}
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd backend && go build ./internal/config/`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "feat: add environment-based configuration"
```

---

### Task 3: Models

**Files:**
- Create: `backend/internal/models/shop.go`
- Create: `backend/internal/models/crawl.go`
- Create: `backend/internal/models/product.go`
- Create: `backend/internal/models/game.go`
- Create: `backend/internal/models/player.go`
- Create: `backend/internal/models/round.go`
- Create: `backend/internal/models/answer.go`

- [ ] **Step 1: Create shop.go**

Create `backend/internal/models/shop.go`:

```go
package models

import (
	"time"

	"github.com/google/uuid"
)

type Shop struct {
	ID             uuid.UUID `json:"id"`
	URL            string    `json:"url"`
	Name           *string   `json:"name"`
	FirstCrawledAt time.Time `json:"first_crawled_at"`
	LastCrawledAt  time.Time `json:"last_crawled_at"`
}
```

- [ ] **Step 2: Create crawl.go**

Create `backend/internal/models/crawl.go`:

```go
package models

import (
	"time"

	"github.com/google/uuid"
)

type CrawlStatus string

const (
	CrawlStatusPending    CrawlStatus = "pending"
	CrawlStatusInProgress CrawlStatus = "in_progress"
	CrawlStatusCompleted  CrawlStatus = "completed"
	CrawlStatusFailed     CrawlStatus = "failed"
)

type Crawl struct {
	ID              uuid.UUID   `json:"id"`
	ShopID          uuid.UUID   `json:"shop_id"`
	SessionID       *uuid.UUID  `json:"session_id"`
	Status          CrawlStatus `json:"status"`
	ProductsFound   int         `json:"products_found"`
	PagesVisited    int         `json:"pages_visited"`
	AIRequestsCount int         `json:"ai_requests_count"`
	ErrorMessage    *string     `json:"error_message"`
	LogFilePath     string      `json:"log_file_path"`
	StartedAt       time.Time   `json:"started_at"`
	FinishedAt      *time.Time  `json:"finished_at"`
	DurationMs      *int        `json:"duration_ms"`
}
```

- [ ] **Step 3: Create product.go**

Create `backend/internal/models/product.go`:

```go
package models

import (
	"time"

	"github.com/google/uuid"
)

type Product struct {
	ID        uuid.UUID `json:"id"`
	ShopID    uuid.UUID `json:"shop_id"`
	CrawlID   uuid.UUID `json:"crawl_id"`
	Name      string    `json:"name"`
	Price     float64   `json:"price"`
	ImageURL  string    `json:"image_url"`
	SourceURL string    `json:"source_url"`
	CreatedAt time.Time `json:"created_at"`
}
```

- [ ] **Step 4: Create game.go**

Create `backend/internal/models/game.go`:

```go
package models

import (
	"time"

	"github.com/google/uuid"
)

type GameMode string

const (
	GameModeComparison GameMode = "comparison"
	GameModeGuess      GameMode = "guess"
)

type GameStatus string

const (
	GameStatusCrawling   GameStatus = "crawling"
	GameStatusReady      GameStatus = "ready"
	GameStatusLobby      GameStatus = "lobby"
	GameStatusInProgress GameStatus = "in_progress"
	GameStatusFinished   GameStatus = "finished"
)

type GameSession struct {
	ID          uuid.UUID  `json:"id"`
	RoomCode    *string    `json:"room_code"`
	HostNick    string     `json:"host_nick"`
	ShopID      uuid.UUID  `json:"shop_id"`
	GameMode    GameMode   `json:"game_mode"`
	RoundsTotal int        `json:"rounds_total"`
	Status      GameStatus `json:"status"`
	CrawlID     *uuid.UUID `json:"crawl_id"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
```

- [ ] **Step 5: Create player.go**

Create `backend/internal/models/player.go`:

```go
package models

import (
	"time"

	"github.com/google/uuid"
)

type Player struct {
	ID        uuid.UUID `json:"id"`
	SessionID uuid.UUID `json:"session_id"`
	Nick      string    `json:"nick"`
	JoinedAt  time.Time `json:"joined_at"`
	IsHost    bool      `json:"is_host"`
}
```

- [ ] **Step 6: Create round.go**

Create `backend/internal/models/round.go`:

```go
package models

import (
	"github.com/google/uuid"
)

type RoundType string

const (
	RoundTypeComparison RoundType = "comparison"
	RoundTypeGuess      RoundType = "guess"
)

type Round struct {
	ID              uuid.UUID  `json:"id"`
	SessionID       uuid.UUID  `json:"session_id"`
	RoundNumber     int        `json:"round_number"`
	RoundType       RoundType  `json:"round_type"`
	ProductAID      uuid.UUID  `json:"product_a_id"`
	ProductBID      *uuid.UUID `json:"product_b_id"`
	CorrectAnswer   string     `json:"correct_answer"`
	DifficultyScore int        `json:"difficulty_score"`
}
```

- [ ] **Step 7: Create answer.go**

Create `backend/internal/models/answer.go`:

```go
package models

import (
	"time"

	"github.com/google/uuid"
)

type Answer struct {
	ID           uuid.UUID `json:"id"`
	RoundID      uuid.UUID `json:"round_id"`
	PlayerID     uuid.UUID `json:"player_id"`
	Answer       string    `json:"answer"`
	IsCorrect    bool      `json:"is_correct"`
	PointsEarned int       `json:"points_earned"`
	AnsweredAt   time.Time `json:"answered_at"`
}
```

- [ ] **Step 8: Verify all models compile**

Run: `cd backend && go build ./internal/models/`
Expected: No errors.

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "feat: add all domain models (shop, crawl, product, game, player, round, answer)"
```

---

### Task 4: SQL Migrations

**Files:**
- Create: `backend/migrations/001_create_shops.up.sql`
- Create: `backend/migrations/001_create_shops.down.sql`
- Create: `backend/migrations/002_create_game_sessions.up.sql`
- Create: `backend/migrations/002_create_game_sessions.down.sql`
- Create: `backend/migrations/003_create_crawls.up.sql`
- Create: `backend/migrations/003_create_crawls.down.sql`
- Create: `backend/migrations/004_create_products.up.sql`
- Create: `backend/migrations/004_create_products.down.sql`
- Create: `backend/migrations/005_create_players.up.sql`
- Create: `backend/migrations/005_create_players.down.sql`
- Create: `backend/migrations/006_create_rounds.up.sql`
- Create: `backend/migrations/006_create_rounds.down.sql`
- Create: `backend/migrations/007_create_answers.up.sql`
- Create: `backend/migrations/007_create_answers.down.sql`

- [ ] **Step 1: Create 001_create_shops**

Create `backend/migrations/001_create_shops.up.sql`:

```sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE shops (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    url TEXT NOT NULL UNIQUE,
    name TEXT,
    first_crawled_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_crawled_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

Create `backend/migrations/001_create_shops.down.sql`:

```sql
DROP TABLE IF EXISTS shops;
```

- [ ] **Step 2: Create 002_create_game_sessions**

Note: `game_sessions` is created WITHOUT `crawl_id` here. The `crawl_id` column is added via ALTER in migration 003 (after the `crawls` table exists) to resolve the circular FK dependency.

Create `backend/migrations/002_create_game_sessions.up.sql`:

```sql
CREATE TYPE game_mode AS ENUM ('comparison', 'guess');
CREATE TYPE game_status AS ENUM ('crawling', 'ready', 'lobby', 'in_progress', 'finished');

CREATE TABLE game_sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    room_code VARCHAR(6),
    host_nick VARCHAR(50) NOT NULL,
    shop_id UUID NOT NULL REFERENCES shops(id),
    game_mode game_mode NOT NULL,
    rounds_total INT NOT NULL DEFAULT 10,
    status game_status NOT NULL DEFAULT 'crawling',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_game_sessions_room_code ON game_sessions(room_code) WHERE room_code IS NOT NULL;
```

Create `backend/migrations/002_create_game_sessions.down.sql`:

```sql
DROP TABLE IF EXISTS game_sessions;
DROP TYPE IF EXISTS game_status;
DROP TYPE IF EXISTS game_mode;
```

- [ ] **Step 3: Create 003_create_crawls (includes ALTER for game_sessions.crawl_id)**

This migration creates the `crawls` table AND adds `crawl_id` FK column to `game_sessions` (resolving the circular dependency).

Create `backend/migrations/003_create_crawls.up.sql`:

```sql
CREATE TYPE crawl_status AS ENUM ('pending', 'in_progress', 'completed', 'failed');

CREATE TABLE crawls (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    shop_id UUID NOT NULL REFERENCES shops(id),
    session_id UUID REFERENCES game_sessions(id),
    status crawl_status NOT NULL DEFAULT 'pending',
    products_found INT NOT NULL DEFAULT 0,
    pages_visited INT NOT NULL DEFAULT 0,
    ai_requests_count INT NOT NULL DEFAULT 0,
    error_message TEXT,
    log_file_path TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    duration_ms INT
);

ALTER TABLE game_sessions ADD COLUMN crawl_id UUID REFERENCES crawls(id);
```

Create `backend/migrations/003_create_crawls.down.sql`:

```sql
ALTER TABLE game_sessions DROP COLUMN IF EXISTS crawl_id;
DROP TABLE IF EXISTS crawls;
DROP TYPE IF EXISTS crawl_status;
```

- [ ] **Step 4: Create 004_create_products**

Create `backend/migrations/004_create_products.up.sql`:

```sql
CREATE TABLE products (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    shop_id UUID NOT NULL REFERENCES shops(id),
    crawl_id UUID NOT NULL REFERENCES crawls(id),
    name TEXT NOT NULL,
    price DECIMAL(12, 2) NOT NULL CHECK (price > 0),
    image_url TEXT NOT NULL DEFAULT '',
    source_url TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_products_shop_id ON products(shop_id);
CREATE INDEX idx_products_crawl_id ON products(crawl_id);
```

Create `backend/migrations/004_create_products.down.sql`:

```sql
DROP TABLE IF EXISTS products;
```

- [ ] **Step 5: Create 005_create_players**

Create `backend/migrations/005_create_players.up.sql`:

```sql
CREATE TABLE players (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id UUID NOT NULL REFERENCES game_sessions(id),
    nick VARCHAR(50) NOT NULL,
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_host BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX idx_players_session_id ON players(session_id);
```

Create `backend/migrations/005_create_players.down.sql`:

```sql
DROP TABLE IF EXISTS players;
```

- [ ] **Step 6: Create 006_create_rounds**

Create `backend/migrations/006_create_rounds.up.sql`:

```sql
CREATE TYPE round_type AS ENUM ('comparison', 'guess');

CREATE TABLE rounds (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id UUID NOT NULL REFERENCES game_sessions(id),
    round_number INT NOT NULL,
    round_type round_type NOT NULL,
    product_a_id UUID NOT NULL REFERENCES products(id),
    product_b_id UUID REFERENCES products(id),
    correct_answer TEXT NOT NULL,
    difficulty_score INT NOT NULL DEFAULT 1,
    UNIQUE(session_id, round_number)
);

CREATE INDEX idx_rounds_session_id ON rounds(session_id);
```

Create `backend/migrations/006_create_rounds.down.sql`:

```sql
DROP TABLE IF EXISTS rounds;
DROP TYPE IF EXISTS round_type;
```

- [ ] **Step 7: Create 007_create_answers**

Create `backend/migrations/007_create_answers.up.sql`:

```sql
CREATE TABLE answers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    round_id UUID NOT NULL REFERENCES rounds(id),
    player_id UUID NOT NULL REFERENCES players(id),
    answer TEXT NOT NULL,
    is_correct BOOLEAN NOT NULL,
    points_earned INT NOT NULL DEFAULT 0,
    answered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(round_id, player_id)
);

CREATE INDEX idx_answers_round_id ON answers(round_id);
CREATE INDEX idx_answers_player_id ON answers(player_id);
```

Create `backend/migrations/007_create_answers.down.sql`:

```sql
DROP TABLE IF EXISTS answers;
```

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "feat: add all SQL migrations (shops, sessions, crawls, products, players, rounds, answers)"
```

---

### Task 5: DB Connection & Migration Runner

**Files:**
- Create: `backend/internal/store/db.go`
- Modify: `backend/cmd/server/main.go`

- [ ] **Step 1: Create db.go**

Create `backend/internal/store/db.go`:

```go
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func ConnectDB(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return db, nil
}

func RunMigrations(db *sql.DB, migrationsPath string) error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("creating migration driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://"+migrationsPath,
		"postgres",
		driver,
	)
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("running migrations: %w", err)
	}

	return nil
}
```

- [ ] **Step 2: Update main.go to connect and migrate**

Update `backend/cmd/server/main.go`:

```go
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

	db, err := store.ConnectDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := store.RunMigrations(db, "../migrations"); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	fmt.Printf("Server ready on port %s\n", cfg.ServerPort)
	os.Exit(0)
}
```

- [ ] **Step 3: Start PostgreSQL and verify migration**

Run:
```bash
docker compose up -d postgres
sleep 2
cd backend && go run ./cmd/server/
```
Expected: "Server ready on port 8080" - migrations applied successfully.

- [ ] **Step 4: Verify tables exist**

Run:
```bash
docker compose exec postgres psql -U hmys -d howmuchyousay -c "\dt"
```
Expected: All 7 tables listed (shops, game_sessions, crawls, products, players, rounds, answers).

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: add DB connection, migration runner, wire up in main.go"
```

---

### Task 6: Test Helper

**Files:**
- Create: `backend/internal/store/testhelper_test.go`

- [ ] **Step 1: Create test helper**

Create `backend/internal/store/testhelper_test.go`:

```go
package store_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/jzy/howmuchyousay/internal/models"
	"github.com/jzy/howmuchyousay/internal/store"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable"
	}

	db, err := store.ConnectDB(dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	if err := store.RunMigrations(db, "../../migrations"); err != nil {
		db.Close()
		t.Fatalf("Failed to run migrations on test database: %v", err)
	}

	t.Cleanup(func() {
		cleanupTestDB(t, db)
		db.Close()
	})

	return db
}

func cleanupTestDB(t *testing.T, db *sql.DB) {
	t.Helper()

	tables := []string{"answers", "rounds", "players", "products", "crawls", "game_sessions", "shops"}
	for _, table := range tables {
		_, err := db.Exec("DELETE FROM " + table)
		if err != nil {
			t.Logf("Warning: failed to clean table %s: %v", table, err)
		}
	}
}

// createTestShop creates a shop for testing purposes.
func createTestShop(t *testing.T, db *sql.DB, url string) *models.Shop {
	t.Helper()
	ctx := context.Background()
	shopStore := store.NewShopStore(db)
	shop, err := shopStore.Create(ctx, url)
	require.NoError(t, err)
	return shop
}

// createTestShopAndCrawl creates a shop and a crawl (no session) for testing.
func createTestShopAndCrawl(t *testing.T, db *sql.DB, url string) (*models.Shop, *models.Crawl) {
	t.Helper()
	ctx := context.Background()
	shop := createTestShop(t, db, url)
	crawlStore := store.NewCrawlStore(db)
	crawl, err := crawlStore.Create(ctx, shop.ID, nil, "/tmp/test.log")
	require.NoError(t, err)
	return shop, crawl
}

// createTestSession creates a shop and a game session for testing.
func createTestSession(t *testing.T, db *sql.DB, shopURL string) (*models.Shop, *models.GameSession) {
	t.Helper()
	ctx := context.Background()
	shop := createTestShop(t, db, shopURL)
	gameStore := store.NewGameStore(db)
	session, err := gameStore.Create(ctx, shop.ID, "TestHost", models.GameModeComparison, 10)
	require.NoError(t, err)
	return shop, session
}

// createTestProducts creates a shop, session, crawl, and N products for testing.
func createTestProducts(t *testing.T, db *sql.DB, shopURL string, count int) (*models.Shop, *models.GameSession, []models.Product) {
	t.Helper()
	ctx := context.Background()
	shop, session := createTestSession(t, db, shopURL)
	crawlStore := store.NewCrawlStore(db)
	crawl, err := crawlStore.Create(ctx, shop.ID, nil, "/tmp/test.log")
	require.NoError(t, err)
	productStore := store.NewProductStore(db)
	var products []models.Product
	for i := 0; i < count; i++ {
		p, err := productStore.Create(ctx, shop.ID, crawl.ID,
			fmt.Sprintf("Product %d", i), float64((i+1)*100), "", "")
		require.NoError(t, err)
		products = append(products, *p)
	}
	return shop, session, products
}

// createTestRound creates a full setup: shop, session, 2 products, 1 comparison round, 1 player.
func createTestRound(t *testing.T, db *sql.DB, shopURL string) (*models.GameSession, *models.Round, *models.Player) {
	t.Helper()
	ctx := context.Background()
	_, session, products := createTestProducts(t, db, shopURL, 2)
	productBID := products[1].ID
	roundStore := store.NewRoundStore(db)
	round, err := roundStore.Create(ctx, session.ID, 1, models.RoundTypeComparison, products[0].ID, &productBID, "b", 2)
	require.NoError(t, err)
	playerStore := store.NewPlayerStore(db)
	player, err := playerStore.Create(ctx, session.ID, "TestPlayer", true)
	require.NoError(t, err)
	return session, round, player
}
```

- [ ] **Step 2: Start test database and verify helper works**

Run:
```bash
docker compose up -d postgres-test
sleep 2
cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/store/ -v -run TestNothing -count=1
```
Expected: `ok` (no tests to run, but compilation + DB connection works).

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "feat: add test helper for store integration tests"
```

---

### Task 7: Shop Store

**Files:**
- Create: `backend/internal/store/shop_store.go`
- Create: `backend/internal/store/shop_store_test.go`

- [ ] **Step 1: Write failing tests for ShopStore**

Create `backend/internal/store/shop_store_test.go`:

```go
package store_test

import (
	"context"
	"testing"

	"github.com/jzy/howmuchyousay/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShopStore_Create(t *testing.T) {
	db := setupTestDB(t)
	s := store.NewShopStore(db)
	ctx := context.Background()

	shop, err := s.Create(ctx, "https://mediaexpert.pl")
	require.NoError(t, err)
	assert.NotEmpty(t, shop.ID)
	assert.Equal(t, "https://mediaexpert.pl", shop.URL)
	assert.Nil(t, shop.Name)
}

func TestShopStore_GetByURL(t *testing.T) {
	db := setupTestDB(t)
	s := store.NewShopStore(db)
	ctx := context.Background()

	created, err := s.Create(ctx, "https://rossmann.pl")
	require.NoError(t, err)

	found, err := s.GetByURL(ctx, "https://rossmann.pl")
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)
	assert.Equal(t, "https://rossmann.pl", found.URL)
}

func TestShopStore_GetByURL_NotFound(t *testing.T) {
	db := setupTestDB(t)
	s := store.NewShopStore(db)
	ctx := context.Background()

	found, err := s.GetByURL(ctx, "https://nonexistent.pl")
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestShopStore_GetByID(t *testing.T) {
	db := setupTestDB(t)
	s := store.NewShopStore(db)
	ctx := context.Background()

	created, err := s.Create(ctx, "https://allegro.pl")
	require.NoError(t, err)

	found, err := s.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, created.ID, found.ID)
}

func TestShopStore_UpdateLastCrawled(t *testing.T) {
	db := setupTestDB(t)
	s := store.NewShopStore(db)
	ctx := context.Background()

	shop, err := s.Create(ctx, "https://empik.com")
	require.NoError(t, err)

	err = s.UpdateLastCrawled(ctx, shop.ID)
	require.NoError(t, err)

	updated, err := s.GetByID(ctx, shop.ID)
	require.NoError(t, err)
	assert.True(t, updated.LastCrawledAt.After(shop.LastCrawledAt) || updated.LastCrawledAt.Equal(shop.LastCrawledAt))
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/store/ -v -run TestShopStore -count=1`
Expected: FAIL - `store.NewShopStore` undefined.

- [ ] **Step 3: Implement ShopStore**

Create `backend/internal/store/shop_store.go`:

```go
package store

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/jzy/howmuchyousay/internal/models"
)

type ShopStore struct {
	db *sql.DB
}

func NewShopStore(db *sql.DB) *ShopStore {
	return &ShopStore{db: db}
}

func (s *ShopStore) Create(ctx context.Context, url string) (*models.Shop, error) {
	shop := &models.Shop{}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO shops (url) VALUES ($1)
		 RETURNING id, url, name, first_crawled_at, last_crawled_at`,
		url,
	).Scan(&shop.ID, &shop.URL, &shop.Name, &shop.FirstCrawledAt, &shop.LastCrawledAt)
	if err != nil {
		return nil, err
	}
	return shop, nil
}

func (s *ShopStore) GetByURL(ctx context.Context, url string) (*models.Shop, error) {
	shop := &models.Shop{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, url, name, first_crawled_at, last_crawled_at FROM shops WHERE url = $1`,
		url,
	).Scan(&shop.ID, &shop.URL, &shop.Name, &shop.FirstCrawledAt, &shop.LastCrawledAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return shop, nil
}

func (s *ShopStore) GetByID(ctx context.Context, id uuid.UUID) (*models.Shop, error) {
	shop := &models.Shop{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, url, name, first_crawled_at, last_crawled_at FROM shops WHERE id = $1`,
		id,
	).Scan(&shop.ID, &shop.URL, &shop.Name, &shop.FirstCrawledAt, &shop.LastCrawledAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return shop, nil
}

func (s *ShopStore) UpdateLastCrawled(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE shops SET last_crawled_at = NOW() WHERE id = $1`,
		id,
	)
	return err
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/store/ -v -run TestShopStore -count=1`
Expected: All 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: add ShopStore with CRUD operations and tests"
```

---

### Task 8: Crawl Store

**Files:**
- Create: `backend/internal/store/crawl_store.go`
- Create: `backend/internal/store/crawl_store_test.go`

- [ ] **Step 1: Write failing tests for CrawlStore**

Create `backend/internal/store/crawl_store_test.go`:

```go
package store_test

import (
	"context"
	"testing"

	"github.com/jzy/howmuchyousay/internal/models"
	"github.com/jzy/howmuchyousay/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCrawlStore_Create(t *testing.T) {
	db := setupTestDB(t)
	shopStore := store.NewShopStore(db)
	crawlStore := store.NewCrawlStore(db)
	ctx := context.Background()

	shop, err := shopStore.Create(ctx, "https://mediaexpert.pl")
	require.NoError(t, err)

	crawl, err := crawlStore.Create(ctx, shop.ID, nil, "/tmp/crawl.log")
	require.NoError(t, err)
	assert.NotEmpty(t, crawl.ID)
	assert.Equal(t, shop.ID, crawl.ShopID)
	assert.Nil(t, crawl.SessionID)
	assert.Equal(t, models.CrawlStatusPending, crawl.Status)
	assert.Equal(t, "/tmp/crawl.log", crawl.LogFilePath)
}

func TestCrawlStore_UpdateStatus(t *testing.T) {
	db := setupTestDB(t)
	shopStore := store.NewShopStore(db)
	crawlStore := store.NewCrawlStore(db)
	ctx := context.Background()

	shop, err := shopStore.Create(ctx, "https://example.com")
	require.NoError(t, err)

	crawl, err := crawlStore.Create(ctx, shop.ID, nil, "/tmp/test.log")
	require.NoError(t, err)

	err = crawlStore.UpdateStatus(ctx, crawl.ID, models.CrawlStatusInProgress)
	require.NoError(t, err)

	updated, err := crawlStore.GetByID(ctx, crawl.ID)
	require.NoError(t, err)
	assert.Equal(t, models.CrawlStatusInProgress, updated.Status)
}

func TestCrawlStore_Finish(t *testing.T) {
	db := setupTestDB(t)
	shopStore := store.NewShopStore(db)
	crawlStore := store.NewCrawlStore(db)
	ctx := context.Background()

	shop, err := shopStore.Create(ctx, "https://finish-test.com")
	require.NoError(t, err)

	crawl, err := crawlStore.Create(ctx, shop.ID, nil, "/tmp/finish.log")
	require.NoError(t, err)

	err = crawlStore.Finish(ctx, crawl.ID, models.CrawlStatusCompleted, 25, 10, 5, nil)
	require.NoError(t, err)

	finished, err := crawlStore.GetByID(ctx, crawl.ID)
	require.NoError(t, err)
	assert.Equal(t, models.CrawlStatusCompleted, finished.Status)
	assert.Equal(t, 25, finished.ProductsFound)
	assert.Equal(t, 10, finished.PagesVisited)
	assert.Equal(t, 5, finished.AIRequestsCount)
	assert.NotNil(t, finished.FinishedAt)
	assert.NotNil(t, finished.DurationMs)
	assert.Nil(t, finished.ErrorMessage)
}

func TestCrawlStore_FinishWithError(t *testing.T) {
	db := setupTestDB(t)
	shopStore := store.NewShopStore(db)
	crawlStore := store.NewCrawlStore(db)
	ctx := context.Background()

	shop, err := shopStore.Create(ctx, "https://error-test.com")
	require.NoError(t, err)

	crawl, err := crawlStore.Create(ctx, shop.ID, nil, "/tmp/error.log")
	require.NoError(t, err)

	errMsg := "timeout exceeded"
	err = crawlStore.Finish(ctx, crawl.ID, models.CrawlStatusFailed, 3, 2, 1, &errMsg)
	require.NoError(t, err)

	finished, err := crawlStore.GetByID(ctx, crawl.ID)
	require.NoError(t, err)
	assert.Equal(t, models.CrawlStatusFailed, finished.Status)
	require.NotNil(t, finished.ErrorMessage)
	assert.Equal(t, "timeout exceeded", *finished.ErrorMessage)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/store/ -v -run TestCrawlStore -count=1`
Expected: FAIL - `store.NewCrawlStore` undefined.

- [ ] **Step 3: Implement CrawlStore**

Create `backend/internal/store/crawl_store.go`:

```go
package store

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/jzy/howmuchyousay/internal/models"
)

type CrawlStore struct {
	db *sql.DB
}

func NewCrawlStore(db *sql.DB) *CrawlStore {
	return &CrawlStore{db: db}
}

func (s *CrawlStore) Create(ctx context.Context, shopID uuid.UUID, sessionID *uuid.UUID, logFilePath string) (*models.Crawl, error) {
	crawl := &models.Crawl{}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO crawls (shop_id, session_id, log_file_path)
		 VALUES ($1, $2, $3)
		 RETURNING id, shop_id, session_id, status, products_found, pages_visited,
		           ai_requests_count, error_message, log_file_path, started_at, finished_at, duration_ms`,
		shopID, sessionID, logFilePath,
	).Scan(
		&crawl.ID, &crawl.ShopID, &crawl.SessionID, &crawl.Status,
		&crawl.ProductsFound, &crawl.PagesVisited, &crawl.AIRequestsCount,
		&crawl.ErrorMessage, &crawl.LogFilePath, &crawl.StartedAt,
		&crawl.FinishedAt, &crawl.DurationMs,
	)
	if err != nil {
		return nil, err
	}
	return crawl, nil
}

func (s *CrawlStore) GetByID(ctx context.Context, id uuid.UUID) (*models.Crawl, error) {
	crawl := &models.Crawl{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, shop_id, session_id, status, products_found, pages_visited,
		        ai_requests_count, error_message, log_file_path, started_at, finished_at, duration_ms
		 FROM crawls WHERE id = $1`,
		id,
	).Scan(
		&crawl.ID, &crawl.ShopID, &crawl.SessionID, &crawl.Status,
		&crawl.ProductsFound, &crawl.PagesVisited, &crawl.AIRequestsCount,
		&crawl.ErrorMessage, &crawl.LogFilePath, &crawl.StartedAt,
		&crawl.FinishedAt, &crawl.DurationMs,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return crawl, nil
}

func (s *CrawlStore) UpdateStatus(ctx context.Context, id uuid.UUID, status models.CrawlStatus) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE crawls SET status = $1 WHERE id = $2`,
		status, id,
	)
	return err
}

func (s *CrawlStore) Finish(ctx context.Context, id uuid.UUID, status models.CrawlStatus, productsFound, pagesVisited, aiRequests int, errorMessage *string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE crawls SET
			status = $1,
			products_found = $2,
			pages_visited = $3,
			ai_requests_count = $4,
			error_message = $5,
			finished_at = NOW(),
			duration_ms = EXTRACT(EPOCH FROM (NOW() - started_at))::INT * 1000
		 WHERE id = $6`,
		status, productsFound, pagesVisited, aiRequests, errorMessage, id,
	)
	return err
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/store/ -v -run TestCrawlStore -count=1`
Expected: All 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: add CrawlStore with create, status update, finish and tests"
```

---

### Task 9: Product Store

**Files:**
- Create: `backend/internal/store/product_store.go`
- Create: `backend/internal/store/product_store_test.go`

- [ ] **Step 1: Write failing tests for ProductStore**

Create `backend/internal/store/product_store_test.go`:

```go
package store_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/jzy/howmuchyousay/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProductStore_Create(t *testing.T) {
	db := setupTestDB(t)
	productStore := store.NewProductStore(db)
	ctx := context.Background()

	shop, crawl := createTestShopAndCrawl(t, db, "https://product-test.com")

	product, err := productStore.Create(ctx, shop.ID, crawl.ID, "Laptop Dell XPS 15", 5999.99, "https://img.com/dell.jpg", "https://product-test.com/dell-xps")
	require.NoError(t, err)
	assert.NotEmpty(t, product.ID)
	assert.Equal(t, "Laptop Dell XPS 15", product.Name)
	assert.Equal(t, 5999.99, product.Price)
	assert.Equal(t, "https://img.com/dell.jpg", product.ImageURL)
}

func TestProductStore_GetByShopID(t *testing.T) {
	db := setupTestDB(t)
	productStore := store.NewProductStore(db)
	ctx := context.Background()

	shop, crawl := createTestShopAndCrawl(t, db, "https://list-test.com")

	_, err := productStore.Create(ctx, shop.ID, crawl.ID, "Product A", 100.00, "", "")
	require.NoError(t, err)
	_, err = productStore.Create(ctx, shop.ID, crawl.ID, "Product B", 200.00, "", "")
	require.NoError(t, err)
	_, err = productStore.Create(ctx, shop.ID, crawl.ID, "Product C", 300.00, "", "")
	require.NoError(t, err)

	products, err := productStore.GetByShopID(ctx, shop.ID)
	require.NoError(t, err)
	assert.Len(t, products, 3)
}

func TestProductStore_CountByShopID(t *testing.T) {
	db := setupTestDB(t)
	productStore := store.NewProductStore(db)
	ctx := context.Background()

	shop, crawl := createTestShopAndCrawl(t, db, "https://count-test.com")

	_, err := productStore.Create(ctx, shop.ID, crawl.ID, "Item 1", 50.00, "", "")
	require.NoError(t, err)
	_, err = productStore.Create(ctx, shop.ID, crawl.ID, "Item 2", 75.00, "", "")
	require.NoError(t, err)

	count, err := productStore.CountByShopID(ctx, shop.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestProductStore_GetRandomByShopID(t *testing.T) {
	db := setupTestDB(t)
	productStore := store.NewProductStore(db)
	ctx := context.Background()

	shop, crawl := createTestShopAndCrawl(t, db, "https://random-test.com")

	for i := 0; i < 10; i++ {
		_, err := productStore.Create(ctx, shop.ID, crawl.ID, fmt.Sprintf("Product %d", i), float64(i+1)*10.0, "", "")
		require.NoError(t, err)
	}

	products, err := productStore.GetRandomByShopID(ctx, shop.ID, 5)
	require.NoError(t, err)
	assert.Len(t, products, 5)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/store/ -v -run TestProductStore -count=1`
Expected: FAIL - `store.NewProductStore` undefined.

- [ ] **Step 3: Implement ProductStore**

Create `backend/internal/store/product_store.go`:

```go
package store

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/jzy/howmuchyousay/internal/models"
)

type ProductStore struct {
	db *sql.DB
}

func NewProductStore(db *sql.DB) *ProductStore {
	return &ProductStore{db: db}
}

func (s *ProductStore) Create(ctx context.Context, shopID, crawlID uuid.UUID, name string, price float64, imageURL, sourceURL string) (*models.Product, error) {
	product := &models.Product{}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO products (shop_id, crawl_id, name, price, image_url, source_url)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, shop_id, crawl_id, name, price, image_url, source_url, created_at`,
		shopID, crawlID, name, price, imageURL, sourceURL,
	).Scan(&product.ID, &product.ShopID, &product.CrawlID, &product.Name,
		&product.Price, &product.ImageURL, &product.SourceURL, &product.CreatedAt)
	if err != nil {
		return nil, err
	}
	return product, nil
}

func (s *ProductStore) GetByShopID(ctx context.Context, shopID uuid.UUID) ([]models.Product, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, shop_id, crawl_id, name, price, image_url, source_url, created_at
		 FROM products WHERE shop_id = $1
		 ORDER BY created_at DESC`,
		shopID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []models.Product
	for rows.Next() {
		var p models.Product
		if err := rows.Scan(&p.ID, &p.ShopID, &p.CrawlID, &p.Name, &p.Price,
			&p.ImageURL, &p.SourceURL, &p.CreatedAt); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, rows.Err()
}

func (s *ProductStore) CountByShopID(ctx context.Context, shopID uuid.UUID) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM products WHERE shop_id = $1`,
		shopID,
	).Scan(&count)
	return count, err
}

func (s *ProductStore) GetRandomByShopID(ctx context.Context, shopID uuid.UUID, limit int) ([]models.Product, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, shop_id, crawl_id, name, price, image_url, source_url, created_at
		 FROM products WHERE shop_id = $1
		 ORDER BY RANDOM()
		 LIMIT $2`,
		shopID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []models.Product
	for rows.Next() {
		var p models.Product
		if err := rows.Scan(&p.ID, &p.ShopID, &p.CrawlID, &p.Name, &p.Price,
			&p.ImageURL, &p.SourceURL, &p.CreatedAt); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, rows.Err()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/store/ -v -run TestProductStore -count=1`
Expected: All 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: add ProductStore with CRUD, random selection, and tests"
```

---

### Task 10: Game Session Store

**Files:**
- Create: `backend/internal/store/game_store.go`
- Create: `backend/internal/store/game_store_test.go`

- [ ] **Step 1: Write failing tests for GameStore**

Create `backend/internal/store/game_store_test.go`:

```go
package store_test

import (
	"context"
	"testing"

	"github.com/jzy/howmuchyousay/internal/models"
	"github.com/jzy/howmuchyousay/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGameStore_Create(t *testing.T) {
	db := setupTestDB(t)
	gameStore := store.NewGameStore(db)
	ctx := context.Background()

	shop := createTestShop(t, db, "https://game-test.com")

	session, err := gameStore.Create(ctx, shop.ID, "Player1", models.GameModeComparison, 10)
	require.NoError(t, err)
	assert.NotEmpty(t, session.ID)
	assert.Nil(t, session.RoomCode)
	assert.Equal(t, "Player1", session.HostNick)
	assert.Equal(t, models.GameModeComparison, session.GameMode)
	assert.Equal(t, 10, session.RoundsTotal)
	assert.Equal(t, models.GameStatusCrawling, session.Status)
}

func TestGameStore_CreateWithRoomCode(t *testing.T) {
	db := setupTestDB(t)
	gameStore := store.NewGameStore(db)
	ctx := context.Background()

	shop := createTestShop(t, db, "https://room-test.com")
	roomCode := "A3K9F2"

	session, err := gameStore.CreateWithRoom(ctx, shop.ID, "Host1", models.GameModeGuess, 10, roomCode)
	require.NoError(t, err)
	require.NotNil(t, session.RoomCode)
	assert.Equal(t, "A3K9F2", *session.RoomCode)
}

func TestGameStore_GetByID(t *testing.T) {
	db := setupTestDB(t)
	gameStore := store.NewGameStore(db)
	ctx := context.Background()

	shop := createTestShop(t, db, "https://getbyid-test.com")

	created, err := gameStore.Create(ctx, shop.ID, "P1", models.GameModeGuess, 10)
	require.NoError(t, err)

	found, err := gameStore.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, created.ID, found.ID)
}

func TestGameStore_GetByRoomCode(t *testing.T) {
	db := setupTestDB(t)
	gameStore := store.NewGameStore(db)
	ctx := context.Background()

	shop := createTestShop(t, db, "https://roomcode-test.com")

	created, err := gameStore.CreateWithRoom(ctx, shop.ID, "Host", models.GameModeComparison, 10, "XYZ123")
	require.NoError(t, err)

	found, err := gameStore.GetByRoomCode(ctx, "XYZ123")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, created.ID, found.ID)
}

func TestGameStore_UpdateStatus(t *testing.T) {
	db := setupTestDB(t)
	gameStore := store.NewGameStore(db)
	ctx := context.Background()

	shop := createTestShop(t, db, "https://status-test.com")

	session, err := gameStore.Create(ctx, shop.ID, "P1", models.GameModeGuess, 10)
	require.NoError(t, err)

	err = gameStore.UpdateStatus(ctx, session.ID, models.GameStatusInProgress)
	require.NoError(t, err)

	updated, err := gameStore.GetByID(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, models.GameStatusInProgress, updated.Status)
}

func TestGameStore_SetCrawlID(t *testing.T) {
	db := setupTestDB(t)
	gameStore := store.NewGameStore(db)
	crawlStore := store.NewCrawlStore(db)
	ctx := context.Background()

	shop := createTestShop(t, db, "https://setcrawl-test.com")

	session, err := gameStore.Create(ctx, shop.ID, "P1", models.GameModeGuess, 10)
	require.NoError(t, err)

	crawl, err := crawlStore.Create(ctx, shop.ID, &session.ID, "/tmp/crawl.log")
	require.NoError(t, err)

	err = gameStore.SetCrawlID(ctx, session.ID, crawl.ID)
	require.NoError(t, err)

	updated, err := gameStore.GetByID(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, updated.CrawlID)
	assert.Equal(t, crawl.ID, *updated.CrawlID)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/store/ -v -run TestGameStore -count=1`
Expected: FAIL - `store.NewGameStore` undefined.

- [ ] **Step 3: Implement GameStore**

Create `backend/internal/store/game_store.go`:

```go
package store

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/jzy/howmuchyousay/internal/models"
)

type GameStore struct {
	db *sql.DB
}

func NewGameStore(db *sql.DB) *GameStore {
	return &GameStore{db: db}
}

func (s *GameStore) Create(ctx context.Context, shopID uuid.UUID, hostNick string, mode models.GameMode, roundsTotal int) (*models.GameSession, error) {
	session := &models.GameSession{}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO game_sessions (shop_id, host_nick, game_mode, rounds_total)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, room_code, host_nick, shop_id, game_mode, rounds_total, status, crawl_id, created_at, updated_at`,
		shopID, hostNick, mode, roundsTotal,
	).Scan(&session.ID, &session.RoomCode, &session.HostNick, &session.ShopID,
		&session.GameMode, &session.RoundsTotal, &session.Status, &session.CrawlID,
		&session.CreatedAt, &session.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (s *GameStore) CreateWithRoom(ctx context.Context, shopID uuid.UUID, hostNick string, mode models.GameMode, roundsTotal int, roomCode string) (*models.GameSession, error) {
	session := &models.GameSession{}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO game_sessions (shop_id, host_nick, game_mode, rounds_total, room_code, status)
		 VALUES ($1, $2, $3, $4, $5, 'lobby')
		 RETURNING id, room_code, host_nick, shop_id, game_mode, rounds_total, status, crawl_id, created_at, updated_at`,
		shopID, hostNick, mode, roundsTotal, roomCode,
	).Scan(&session.ID, &session.RoomCode, &session.HostNick, &session.ShopID,
		&session.GameMode, &session.RoundsTotal, &session.Status, &session.CrawlID,
		&session.CreatedAt, &session.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (s *GameStore) GetByID(ctx context.Context, id uuid.UUID) (*models.GameSession, error) {
	session := &models.GameSession{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, room_code, host_nick, shop_id, game_mode, rounds_total, status, crawl_id, created_at, updated_at
		 FROM game_sessions WHERE id = $1`,
		id,
	).Scan(&session.ID, &session.RoomCode, &session.HostNick, &session.ShopID,
		&session.GameMode, &session.RoundsTotal, &session.Status, &session.CrawlID,
		&session.CreatedAt, &session.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (s *GameStore) GetByRoomCode(ctx context.Context, code string) (*models.GameSession, error) {
	session := &models.GameSession{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, room_code, host_nick, shop_id, game_mode, rounds_total, status, crawl_id, created_at, updated_at
		 FROM game_sessions WHERE room_code = $1`,
		code,
	).Scan(&session.ID, &session.RoomCode, &session.HostNick, &session.ShopID,
		&session.GameMode, &session.RoundsTotal, &session.Status, &session.CrawlID,
		&session.CreatedAt, &session.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (s *GameStore) UpdateStatus(ctx context.Context, id uuid.UUID, status models.GameStatus) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE game_sessions SET status = $1, updated_at = NOW() WHERE id = $2`,
		status, id,
	)
	return err
}

func (s *GameStore) SetCrawlID(ctx context.Context, sessionID, crawlID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE game_sessions SET crawl_id = $1, updated_at = NOW() WHERE id = $2`,
		crawlID, sessionID,
	)
	return err
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/store/ -v -run TestGameStore -count=1`
Expected: All 6 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: add GameStore with create, room code, status update, and tests"
```

---

### Task 11: Player Store

**Files:**
- Create: `backend/internal/store/player_store.go`
- Create: `backend/internal/store/player_store_test.go`

- [ ] **Step 1: Write failing tests for PlayerStore**

Create `backend/internal/store/player_store_test.go`:

```go
package store_test

import (
	"context"
	"testing"

	"github.com/jzy/howmuchyousay/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlayerStore_Create(t *testing.T) {
	db := setupTestDB(t)
	playerStore := store.NewPlayerStore(db)
	ctx := context.Background()

	_, session := createTestSession(t, db, "https://player-test.com")

	player, err := playerStore.Create(ctx, session.ID, "Alice", true)
	require.NoError(t, err)
	assert.NotEmpty(t, player.ID)
	assert.Equal(t, "Alice", player.Nick)
	assert.True(t, player.IsHost)
}

func TestPlayerStore_GetBySessionID(t *testing.T) {
	db := setupTestDB(t)
	playerStore := store.NewPlayerStore(db)
	ctx := context.Background()

	_, session := createTestSession(t, db, "https://player-list-test.com")

	_, err := playerStore.Create(ctx, session.ID, "Alice", true)
	require.NoError(t, err)
	_, err = playerStore.Create(ctx, session.ID, "Bob", false)
	require.NoError(t, err)

	players, err := playerStore.GetBySessionID(ctx, session.ID)
	require.NoError(t, err)
	assert.Len(t, players, 2)
}

func TestPlayerStore_CountBySessionID(t *testing.T) {
	db := setupTestDB(t)
	playerStore := store.NewPlayerStore(db)
	ctx := context.Background()

	_, session := createTestSession(t, db, "https://player-count-test.com")

	_, err := playerStore.Create(ctx, session.ID, "Alice", true)
	require.NoError(t, err)
	_, err = playerStore.Create(ctx, session.ID, "Bob", false)
	require.NoError(t, err)
	_, err = playerStore.Create(ctx, session.ID, "Charlie", false)
	require.NoError(t, err)

	count, err := playerStore.CountBySessionID(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/store/ -v -run TestPlayerStore -count=1`
Expected: FAIL - `store.NewPlayerStore` undefined.

- [ ] **Step 3: Implement PlayerStore**

Create `backend/internal/store/player_store.go`:

```go
package store

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/jzy/howmuchyousay/internal/models"
)

type PlayerStore struct {
	db *sql.DB
}

func NewPlayerStore(db *sql.DB) *PlayerStore {
	return &PlayerStore{db: db}
}

func (s *PlayerStore) Create(ctx context.Context, sessionID uuid.UUID, nick string, isHost bool) (*models.Player, error) {
	player := &models.Player{}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO players (session_id, nick, is_host)
		 VALUES ($1, $2, $3)
		 RETURNING id, session_id, nick, joined_at, is_host`,
		sessionID, nick, isHost,
	).Scan(&player.ID, &player.SessionID, &player.Nick, &player.JoinedAt, &player.IsHost)
	if err != nil {
		return nil, err
	}
	return player, nil
}

func (s *PlayerStore) GetBySessionID(ctx context.Context, sessionID uuid.UUID) ([]models.Player, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, nick, joined_at, is_host
		 FROM players WHERE session_id = $1
		 ORDER BY joined_at ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var players []models.Player
	for rows.Next() {
		var p models.Player
		if err := rows.Scan(&p.ID, &p.SessionID, &p.Nick, &p.JoinedAt, &p.IsHost); err != nil {
			return nil, err
		}
		players = append(players, p)
	}
	return players, rows.Err()
}

func (s *PlayerStore) CountBySessionID(ctx context.Context, sessionID uuid.UUID) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM players WHERE session_id = $1`,
		sessionID,
	).Scan(&count)
	return count, err
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/store/ -v -run TestPlayerStore -count=1`
Expected: All 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: add PlayerStore with create, list by session, count, and tests"
```

---

### Task 12: Round Store

**Files:**
- Create: `backend/internal/store/round_store.go`
- Create: `backend/internal/store/round_store_test.go`

- [ ] **Step 1: Write failing tests for RoundStore**

Create `backend/internal/store/round_store_test.go`:

```go
package store_test

import (
	"context"
	"testing"

	"github.com/jzy/howmuchyousay/internal/models"
	"github.com/jzy/howmuchyousay/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoundStore_Create(t *testing.T) {
	db := setupTestDB(t)
	roundStore := store.NewRoundStore(db)
	ctx := context.Background()

	_, session, products := createTestProducts(t, db, "https://round-test.com", 2)
	productBID := products[1].ID

	round, err := roundStore.Create(ctx, session.ID, 1, models.RoundTypeComparison, products[0].ID, &productBID, "b", 2)
	require.NoError(t, err)
	assert.NotEmpty(t, round.ID)
	assert.Equal(t, 1, round.RoundNumber)
	assert.Equal(t, models.RoundTypeComparison, round.RoundType)
	assert.Equal(t, "b", round.CorrectAnswer)
	assert.Equal(t, 2, round.DifficultyScore)
}

func TestRoundStore_CreateGuessRound(t *testing.T) {
	db := setupTestDB(t)
	roundStore := store.NewRoundStore(db)
	ctx := context.Background()

	_, session, products := createTestProducts(t, db, "https://guess-round-test.com", 1)

	round, err := roundStore.Create(ctx, session.ID, 1, models.RoundTypeGuess, products[0].ID, nil, "100.00", 3)
	require.NoError(t, err)
	assert.Nil(t, round.ProductBID)
	assert.Equal(t, models.RoundTypeGuess, round.RoundType)
}

func TestRoundStore_GetBySessionID(t *testing.T) {
	db := setupTestDB(t)
	roundStore := store.NewRoundStore(db)
	ctx := context.Background()

	_, session, products := createTestProducts(t, db, "https://round-list-test.com", 4)
	productBID1 := products[1].ID
	productBID2 := products[3].ID

	_, err := roundStore.Create(ctx, session.ID, 1, models.RoundTypeComparison, products[0].ID, &productBID1, "a", 1)
	require.NoError(t, err)
	_, err = roundStore.Create(ctx, session.ID, 2, models.RoundTypeComparison, products[2].ID, &productBID2, "b", 3)
	require.NoError(t, err)

	rounds, err := roundStore.GetBySessionID(ctx, session.ID)
	require.NoError(t, err)
	assert.Len(t, rounds, 2)
	assert.Equal(t, 1, rounds[0].RoundNumber)
	assert.Equal(t, 2, rounds[1].RoundNumber)
}

func TestRoundStore_GetBySessionAndNumber(t *testing.T) {
	db := setupTestDB(t)
	roundStore := store.NewRoundStore(db)
	ctx := context.Background()

	_, session, products := createTestProducts(t, db, "https://round-number-test.com", 2)
	productBID := products[1].ID

	_, err := roundStore.Create(ctx, session.ID, 1, models.RoundTypeComparison, products[0].ID, &productBID, "a", 2)
	require.NoError(t, err)

	found, err := roundStore.GetBySessionAndNumber(ctx, session.ID, 1)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, 1, found.RoundNumber)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/store/ -v -run TestRoundStore -count=1`
Expected: FAIL - `store.NewRoundStore` undefined.

- [ ] **Step 3: Implement RoundStore**

Create `backend/internal/store/round_store.go`:

```go
package store

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/jzy/howmuchyousay/internal/models"
)

type RoundStore struct {
	db *sql.DB
}

func NewRoundStore(db *sql.DB) *RoundStore {
	return &RoundStore{db: db}
}

func (s *RoundStore) Create(ctx context.Context, sessionID uuid.UUID, roundNumber int, roundType models.RoundType, productAID uuid.UUID, productBID *uuid.UUID, correctAnswer string, difficultyScore int) (*models.Round, error) {
	round := &models.Round{}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO rounds (session_id, round_number, round_type, product_a_id, product_b_id, correct_answer, difficulty_score)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, session_id, round_number, round_type, product_a_id, product_b_id, correct_answer, difficulty_score`,
		sessionID, roundNumber, roundType, productAID, productBID, correctAnswer, difficultyScore,
	).Scan(&round.ID, &round.SessionID, &round.RoundNumber, &round.RoundType,
		&round.ProductAID, &round.ProductBID, &round.CorrectAnswer, &round.DifficultyScore)
	if err != nil {
		return nil, err
	}
	return round, nil
}

func (s *RoundStore) GetBySessionID(ctx context.Context, sessionID uuid.UUID) ([]models.Round, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, round_number, round_type, product_a_id, product_b_id, correct_answer, difficulty_score
		 FROM rounds WHERE session_id = $1
		 ORDER BY round_number ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rounds []models.Round
	for rows.Next() {
		var r models.Round
		if err := rows.Scan(&r.ID, &r.SessionID, &r.RoundNumber, &r.RoundType,
			&r.ProductAID, &r.ProductBID, &r.CorrectAnswer, &r.DifficultyScore); err != nil {
			return nil, err
		}
		rounds = append(rounds, r)
	}
	return rounds, rows.Err()
}

func (s *RoundStore) GetBySessionAndNumber(ctx context.Context, sessionID uuid.UUID, roundNumber int) (*models.Round, error) {
	round := &models.Round{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, session_id, round_number, round_type, product_a_id, product_b_id, correct_answer, difficulty_score
		 FROM rounds WHERE session_id = $1 AND round_number = $2`,
		sessionID, roundNumber,
	).Scan(&round.ID, &round.SessionID, &round.RoundNumber, &round.RoundType,
		&round.ProductAID, &round.ProductBID, &round.CorrectAnswer, &round.DifficultyScore)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return round, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/store/ -v -run TestRoundStore -count=1`
Expected: All 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: add RoundStore with create, list, get by number, and tests"
```

---

### Task 13: Answer Store

**Files:**
- Create: `backend/internal/store/answer_store.go`
- Create: `backend/internal/store/answer_store_test.go`

- [ ] **Step 1: Write failing tests for AnswerStore**

Create `backend/internal/store/answer_store_test.go`:

```go
package store_test

import (
	"context"
	"testing"

	"github.com/jzy/howmuchyousay/internal/models"
	"github.com/jzy/howmuchyousay/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnswerStore_Create(t *testing.T) {
	db := setupTestDB(t)
	answerStore := store.NewAnswerStore(db)
	ctx := context.Background()

	_, round, player := createTestRound(t, db, "https://answer-test.com")

	answer, err := answerStore.Create(ctx, round.ID, player.ID, "b", true, 2)
	require.NoError(t, err)
	assert.NotEmpty(t, answer.ID)
	assert.Equal(t, "b", answer.Answer)
	assert.True(t, answer.IsCorrect)
	assert.Equal(t, 2, answer.PointsEarned)
}

func TestAnswerStore_GetByRoundID(t *testing.T) {
	db := setupTestDB(t)
	answerStore := store.NewAnswerStore(db)
	playerStore := store.NewPlayerStore(db)
	ctx := context.Background()

	session, round, player1 := createTestRound(t, db, "https://answer-list-test.com")

	player2, err := playerStore.Create(ctx, session.ID, "Player2", false)
	require.NoError(t, err)

	_, err = answerStore.Create(ctx, round.ID, player1.ID, "a", false, 0)
	require.NoError(t, err)
	_, err = answerStore.Create(ctx, round.ID, player2.ID, "b", true, 2)
	require.NoError(t, err)

	answers, err := answerStore.GetByRoundID(ctx, round.ID)
	require.NoError(t, err)
	assert.Len(t, answers, 2)
}

func TestAnswerStore_CountByRoundID(t *testing.T) {
	db := setupTestDB(t)
	answerStore := store.NewAnswerStore(db)
	ctx := context.Background()

	_, round, player := createTestRound(t, db, "https://answer-count-test.com")

	_, err := answerStore.Create(ctx, round.ID, player.ID, "a", false, 0)
	require.NoError(t, err)

	count, err := answerStore.CountByRoundID(ctx, round.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestAnswerStore_GetPlayerTotalScore(t *testing.T) {
	db := setupTestDB(t)
	answerStore := store.NewAnswerStore(db)
	roundStore := store.NewRoundStore(db)
	ctx := context.Background()

	_, session, products := createTestProducts(t, db, "https://score-test.com", 4)
	playerStore := store.NewPlayerStore(db)
	player, err := playerStore.Create(ctx, session.ID, "Scorer", true)
	require.NoError(t, err)

	productBID1 := products[1].ID
	productBID2 := products[3].ID

	round1, err := roundStore.Create(ctx, session.ID, 1, models.RoundTypeComparison, products[0].ID, &productBID1, "b", 2)
	require.NoError(t, err)
	round2, err := roundStore.Create(ctx, session.ID, 2, models.RoundTypeComparison, products[2].ID, &productBID2, "a", 3)
	require.NoError(t, err)

	_, err = answerStore.Create(ctx, round1.ID, player.ID, "b", true, 2)
	require.NoError(t, err)
	_, err = answerStore.Create(ctx, round2.ID, player.ID, "a", true, 3)
	require.NoError(t, err)

	total, err := answerStore.GetPlayerTotalScore(ctx, session.ID, player.ID)
	require.NoError(t, err)
	assert.Equal(t, 5, total)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/store/ -v -run TestAnswerStore -count=1`
Expected: FAIL - `store.NewAnswerStore` undefined.

- [ ] **Step 3: Implement AnswerStore**

Create `backend/internal/store/answer_store.go`:

```go
package store

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/jzy/howmuchyousay/internal/models"
)

type AnswerStore struct {
	db *sql.DB
}

func NewAnswerStore(db *sql.DB) *AnswerStore {
	return &AnswerStore{db: db}
}

func (s *AnswerStore) Create(ctx context.Context, roundID, playerID uuid.UUID, answer string, isCorrect bool, pointsEarned int) (*models.Answer, error) {
	a := &models.Answer{}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO answers (round_id, player_id, answer, is_correct, points_earned)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, round_id, player_id, answer, is_correct, points_earned, answered_at`,
		roundID, playerID, answer, isCorrect, pointsEarned,
	).Scan(&a.ID, &a.RoundID, &a.PlayerID, &a.Answer, &a.IsCorrect, &a.PointsEarned, &a.AnsweredAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (s *AnswerStore) GetByRoundID(ctx context.Context, roundID uuid.UUID) ([]models.Answer, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, round_id, player_id, answer, is_correct, points_earned, answered_at
		 FROM answers WHERE round_id = $1
		 ORDER BY answered_at ASC`,
		roundID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var answers []models.Answer
	for rows.Next() {
		var a models.Answer
		if err := rows.Scan(&a.ID, &a.RoundID, &a.PlayerID, &a.Answer,
			&a.IsCorrect, &a.PointsEarned, &a.AnsweredAt); err != nil {
			return nil, err
		}
		answers = append(answers, a)
	}
	return answers, rows.Err()
}

func (s *AnswerStore) CountByRoundID(ctx context.Context, roundID uuid.UUID) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM answers WHERE round_id = $1`,
		roundID,
	).Scan(&count)
	return count, err
}

func (s *AnswerStore) GetPlayerTotalScore(ctx context.Context, sessionID, playerID uuid.UUID) (int, error) {
	var total int
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(a.points_earned), 0)
		 FROM answers a
		 JOIN rounds r ON r.id = a.round_id
		 WHERE r.session_id = $1 AND a.player_id = $2`,
		sessionID, playerID,
	).Scan(&total)
	return total, err
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/store/ -v -run TestAnswerStore -count=1`
Expected: All 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: add AnswerStore with create, list, count, total score, and tests"
```

---

### Task 14: Full Test Suite Run

- [ ] **Step 1: Run all store tests together**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./... -v -count=1`
Expected: All tests PASS (approximately 25 tests across 7 store test files).

- [ ] **Step 2: Run go vet**

Run: `cd backend && go vet ./...`
Expected: No issues.

- [ ] **Step 3: Final commit if any cleanup was needed**

```bash
git add -A
git commit -m "chore: verify full test suite passes, clean up any issues"
```

(Skip this commit if no changes were needed.)
