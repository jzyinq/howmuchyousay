# Plan 4 — HTTP API (Single-Player) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn `cmd/server/main.go` from a stub into a working single-player HTTP server exposing the game end-to-end (create session, poll status, fetch round, submit answer, fetch results) backed by the existing `game`, `store`, and `crawler` packages.

**Architecture:** New `internal/server` package using Gin with struct-based handler injection. `cmd/server/main.go` becomes a thin composition root that wires a `Deps` struct and runs an `http.Server` with SIGINT/SIGTERM graceful shutdown. Crawls run in a fire-and-forget goroutine with `context.Background()` (so HTTP request cancellation does not kill them) and report success/failure by transitioning the session through `crawling → ready → (in_progress → finished)` or `crawling → failed`. `current_round` is a stored column on `game_sessions`, advanced atomically inside the answer handler's pgx transaction — a pattern that survives Plan 5's multiplayer barrier without rework.

**Tech Stack:** Go 1.26, Gin (`github.com/gin-gonic/gin`), pgx/v5, PostgreSQL 16, `slog`, `math/rand/v2`, `httptest`, testify/require, golang-migrate v4. Existing test infrastructure: `postgres-test` service on port 5433, `TEST_DATABASE_URL`, `store.RunMigrations`, `cleanupTestDB` pattern, `-p 1` serial package execution (already in Makefile).

**Spec:** `docs/superpowers/specs/2026-04-10-plan4-http-api-design.md`

**Two notes about the spec that this plan adjusts:**

1. **The `answers` table already has a `UNIQUE(round_id, player_id)` constraint** from migration `007_create_answers.up.sql` (inline, auto-named). Migration 008 does **not** re-add it; duplicate detection in the answer handler relies solely on pgx SQLSTATE `23505` (no constraint-name check required — the answers table has no other unique constraint that could fire on INSERT).
2. **Transaction mechanism:** a shared `store.DBExec` interface is introduced in Task 2. `AnswerStore.Create`, `GameStore.UpdateStatus`, and the new `GameStore.IncrementCurrentRound` take a `DBExec` as their first parameter after `ctx`. Both `*pgxpool.Pool` and `pgx.Tx` satisfy it. Callers that do not need a transaction pass the pool directly.

---

## File Structure (locked in before tasks)

```
backend/
├── cmd/server/
│   └── main.go                          # MODIFIED (Task 13): composition root, http.Server, graceful shutdown
│
├── internal/server/                     # NEW package (created over Tasks 3–13)
│   ├── server.go                        # Handler struct, Deps, New(), Routes()
│   ├── errors.go                        # APIError type + constructors + errorMiddleware
│   ├── errors_test.go                   # Error middleware unit tests
│   ├── dto.go                           # Request/response DTOs with binding tags
│   ├── crawl_runner.go                  # Crawler interface + runCrawlForSession + markSessionFailed
│   ├── rounds.go                        # generateAndPersistRounds helper
│   ├── game_handlers.go                 # CreateGame, GetSession, GetRound, PostAnswer, GetResults
│   ├── testhelper_test.go               # setupTestHandler, setupTestDB, cleanupTestDB, fakeCrawler, seedShopWithProducts
│   ├── create_game_test.go              # CreateGame integration tests
│   ├── get_session_test.go              # GetSession integration tests
│   ├── get_round_test.go                # GetRound integration tests
│   ├── post_answer_test.go              # PostAnswer integration tests
│   └── get_results_test.go              # GetResults integration tests
│
├── internal/models/
│   └── game.go                          # MODIFIED (Task 1): GameStatusFailed, CurrentRound, ErrorMessage
│
├── internal/store/
│   ├── db_exec.go                       # NEW (Task 2): DBExec interface
│   ├── game_store.go                    # MODIFIED (Tasks 1, 2): scan new columns, UpdateStatus(DBExec), IncrementCurrentRound, SetErrorMessage
│   ├── game_store_test.go               # MODIFIED (Tasks 1, 2): new signatures + new column assertions
│   ├── answer_store.go                  # MODIFIED (Task 2): Create takes DBExec
│   └── answer_store_test.go             # MODIFIED (Task 2): pass pool to Create
│
└── migrations/
    ├── 008_plan4_http_api.up.sql        # NEW (Task 1)
    └── 008_plan4_http_api.down.sql      # NEW (Task 1)
```

**Task-to-file mapping at a glance:**
- Task 1: migrations 008 + `internal/models/game.go` + `internal/store/game_store.go` (scans) + existing game_store tests updated
- Task 2: `internal/store/db_exec.go` + `internal/store/game_store.go` (signatures + new methods) + `internal/store/answer_store.go` + both store test files
- Task 3: `internal/server/server.go` + `go get gin`
- Task 4: `internal/server/errors.go` + `internal/server/errors_test.go`
- Task 5: `internal/server/dto.go`
- Task 6: `internal/server/crawl_runner.go` (interface only) + `internal/server/testhelper_test.go`
- Task 7: `internal/server/rounds.go`
- Task 8: `internal/server/game_handlers.go` (CreateGame skip_crawl=true) + `internal/server/create_game_test.go`
- Task 9: `internal/server/crawl_runner.go` (runCrawlForSession, markSessionFailed) + `internal/server/game_handlers.go` (CreateGame skip_crawl=false) + create_game_test.go extended
- Task 10: `internal/server/game_handlers.go` (GetSession) + `internal/server/get_session_test.go`
- Task 11: `internal/server/game_handlers.go` (GetRound) + `internal/server/get_round_test.go`
- Task 12: `internal/server/game_handlers.go` (PostAnswer) + `internal/server/post_answer_test.go`
- Task 13: `internal/server/game_handlers.go` (GetResults) + `internal/server/get_results_test.go` + `cmd/server/main.go`
- Task 14: full test suite, vet, build verification

**TDD rule for this plan:** write the failing test first, run it, see it fail, then implement. Commit after each task passes.

---

## Task 1: Migration 008 + model fields + GameStore scan updates

**Files:**
- Create: `backend/migrations/008_plan4_http_api.up.sql`
- Create: `backend/migrations/008_plan4_http_api.down.sql`
- Modify: `backend/internal/models/game.go`
- Modify: `backend/internal/store/game_store.go` (4 SQL statements + 4 scan sites)
- Modify: `backend/internal/store/game_store_test.go` (extend existing assertions)

- [ ] **Step 1: Write the migration files**

Create `backend/migrations/008_plan4_http_api.up.sql`:

```sql
-- Add "failed" to the game_status enum.
-- PostgreSQL 12+ permits ALTER TYPE ADD VALUE inside a transaction as long as the new
-- value is not referenced in the same transaction. This migration only adds the value;
-- it does not INSERT or UPDATE any row to 'failed', so running under golang-migrate's
-- default transactional wrapping is safe. Both dev and test DBs run postgres:16-alpine.
ALTER TYPE game_status ADD VALUE IF NOT EXISTS 'failed';

-- Error message populated when a session transitions to "failed".
ALTER TABLE game_sessions ADD COLUMN error_message TEXT;

-- Current round tracker. Starts at 1; advances per §9 of the spec.
-- Single-player (Plan 4): advanced inline in the answer handler.
-- Multiplayer (Plan 5): advanced by the barrier service — same column, different trigger.
ALTER TABLE game_sessions ADD COLUMN current_round INT NOT NULL DEFAULT 1;
```

Create `backend/migrations/008_plan4_http_api.down.sql`:

```sql
ALTER TABLE game_sessions DROP COLUMN IF EXISTS current_round;
ALTER TABLE game_sessions DROP COLUMN IF EXISTS error_message;
-- PostgreSQL does not support removing a value from an ENUM without recreating the type.
-- This down migration is best-effort for dev rollbacks.
```

- [ ] **Step 2: Update the `GameSession` model**

In `backend/internal/models/game.go`, add the `GameStatusFailed` constant and two new struct fields. After the existing constants block:

```go
const (
    GameStatusCrawling   GameStatus = "crawling"
    GameStatusReady      GameStatus = "ready"
    GameStatusLobby      GameStatus = "lobby"
    GameStatusInProgress GameStatus = "in_progress"
    GameStatusFinished   GameStatus = "finished"
    GameStatusFailed     GameStatus = "failed"
)

type GameSession struct {
    ID           uuid.UUID  `json:"id"`
    RoomCode     *string    `json:"room_code"`
    HostNick     string     `json:"host_nick"`
    ShopID       uuid.UUID  `json:"shop_id"`
    GameMode     GameMode   `json:"game_mode"`
    RoundsTotal  int        `json:"rounds_total"`
    Status       GameStatus `json:"status"`
    CrawlID      *uuid.UUID `json:"crawl_id"`
    CurrentRound int        `json:"current_round"`
    ErrorMessage *string    `json:"error_message,omitempty"`
    CreatedAt    time.Time  `json:"created_at"`
    UpdatedAt    time.Time  `json:"updated_at"`
}
```

- [ ] **Step 3: Write the failing test — GetByID returns CurrentRound and ErrorMessage fields**

Append to `backend/internal/store/game_store_test.go`:

```go
func TestGameStore_GetByID_NewColumns(t *testing.T) {
    pool := setupTestDB(t)
    gameStore := store.NewGameStore(pool)
    ctx := context.Background()

    shop := createTestShop(t, pool, "https://new-columns-test.com")
    session, err := gameStore.Create(ctx, shop.ID, "P1", models.GameModeComparison, 10)
    require.NoError(t, err)

    found, err := gameStore.GetByID(ctx, session.ID)
    require.NoError(t, err)
    require.NotNil(t, found)
    assert.Equal(t, 1, found.CurrentRound, "new sessions start at round 1")
    assert.Nil(t, found.ErrorMessage, "new sessions have no error message")
}
```

- [ ] **Step 4: Run the test to verify it fails**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/store/ -run TestGameStore_GetByID_NewColumns -v`

Expected: FAIL — either "unknown field CurrentRound" at compile time, or the runtime scan fails because the SELECT doesn't include `current_round`.

- [ ] **Step 5: Update `game_store.go` — all 4 SELECT / RETURNING clauses and scan targets**

Edit each of `Create`, `CreateWithRoom`, `GetByID`, `GetByRoomCode`:

- Change every `SELECT`/`RETURNING` column list from
  ```
  id, room_code, host_nick, shop_id, game_mode, rounds_total, status, crawl_id, created_at, updated_at
  ```
  to
  ```
  id, room_code, host_nick, shop_id, game_mode, rounds_total, status, crawl_id, current_round, error_message, created_at, updated_at
  ```
- Change every `.Scan(...)` call's target list from
  ```go
  &session.ID, &session.RoomCode, &session.HostNick, &session.ShopID,
  &session.GameMode, &session.RoundsTotal, &session.Status, &session.CrawlID,
  &session.CreatedAt, &session.UpdatedAt
  ```
  to
  ```go
  &session.ID, &session.RoomCode, &session.HostNick, &session.ShopID,
  &session.GameMode, &session.RoundsTotal, &session.Status, &session.CrawlID,
  &session.CurrentRound, &session.ErrorMessage, &session.CreatedAt, &session.UpdatedAt
  ```

Do this in all four methods. Leave `UpdateStatus` and `SetCrawlID` (which don't scan the row) alone.

- [ ] **Step 6: Run the new test — expect PASS**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/store/ -run TestGameStore_GetByID_NewColumns -v`

Expected: PASS.

- [ ] **Step 7: Run the full store test package to catch regressions**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/store/ -v`

Expected: all existing store tests still PASS.

- [ ] **Step 8: Commit**

```bash
cd /home/jzy/projects/howmuchyousay
git add backend/migrations/008_plan4_http_api.up.sql backend/migrations/008_plan4_http_api.down.sql backend/internal/models/game.go backend/internal/store/game_store.go backend/internal/store/game_store_test.go
git commit -m "$(cat <<'EOF'
feat(store): add current_round + error_message + failed status

Migration 008 adds the stored current_round column (single source of
truth for which round the session is on, survives multiplayer in Plan 5),
an error_message column for failed-crawl diagnostics, and the 'failed'
game_status enum value. GameSession model and GameStore read paths scan
the new columns.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: DBExec interface + transactional store methods

**Files:**
- Create: `backend/internal/store/db_exec.go`
- Modify: `backend/internal/store/answer_store.go`
- Modify: `backend/internal/store/answer_store_test.go`
- Modify: `backend/internal/store/game_store.go`
- Modify: `backend/internal/store/game_store_test.go`

- [ ] **Step 1: Create the `DBExec` interface**

Create `backend/internal/store/db_exec.go`:

```go
package store

import (
    "context"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgconn"
)

// DBExec is the subset of pgx methods used by store methods that may run
// inside a transaction. Both *pgxpool.Pool and pgx.Tx satisfy it.
//
// Store methods that do not participate in the answer-handler transaction
// keep their existing pool-based signatures.
type DBExec interface {
    Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
    Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
    QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}
```

- [ ] **Step 2: Write the failing test for `AnswerStore.Create(ctx, DBExec, ...)` transactional path**

Append to `backend/internal/store/answer_store_test.go`:

```go
func TestAnswerStore_Create_InTransaction_RollsBack(t *testing.T) {
    pool := setupTestDB(t)
    answerStore := store.NewAnswerStore(pool)
    ctx := context.Background()

    _, round, player := createTestRound(t, pool, "https://answer-tx-test.com")

    tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
    require.NoError(t, err)

    _, err = answerStore.Create(ctx, tx, round.ID, player.ID, "a", false, 0)
    require.NoError(t, err)

    // Rollback — the row must disappear.
    require.NoError(t, tx.Rollback(ctx))

    count, err := answerStore.CountByRoundID(ctx, round.ID)
    require.NoError(t, err)
    assert.Equal(t, 0, count, "answer inserted in rolled-back tx must not persist")
}
```

Also add the import: `"github.com/jackc/pgx/v5"` at the top of `answer_store_test.go`.

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/store/ -run TestAnswerStore_Create_InTransaction_RollsBack -v`

Expected: FAIL at compile — `too many arguments in call to answerStore.Create`.

- [ ] **Step 4: Update `AnswerStore.Create` signature to accept `DBExec`**

Edit `backend/internal/store/answer_store.go`:

```go
func (s *AnswerStore) Create(ctx context.Context, db DBExec, roundID, playerID uuid.UUID, answer string, isCorrect bool, pointsEarned int) (*models.Answer, error) {
    a := &models.Answer{}
    err := db.QueryRow(ctx,
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
```

(The struct and `NewAnswerStore` still hold the pool — it's still used by `GetByRoundID`, `CountByRoundID`, and `GetPlayerTotalScore`, which remain unchanged.)

- [ ] **Step 5: Update existing answer_store_test.go call sites to pass `pool`**

Four call sites in `answer_store_test.go` currently call `answerStore.Create(ctx, round.ID, ...)`. Change each to `answerStore.Create(ctx, pool, round.ID, ...)`:

- `TestAnswerStore_Create` (line 20)
- `TestAnswerStore_GetByRoundID` (lines 39, 41)
- `TestAnswerStore_CountByRoundID` (line 56)
- `TestAnswerStore_GetPlayerTotalScore` (lines 83, 85)

- [ ] **Step 6: Update `GameStore.UpdateStatus` signature to accept `DBExec`**

Edit `backend/internal/store/game_store.go`:

```go
func (s *GameStore) UpdateStatus(ctx context.Context, db DBExec, id uuid.UUID, status models.GameStatus) error {
    _, err := db.Exec(ctx,
        `UPDATE game_sessions SET status = $1, updated_at = NOW() WHERE id = $2`,
        status, id,
    )
    return err
}
```

- [ ] **Step 7: Add `IncrementCurrentRound` and `SetErrorMessage` to `GameStore`**

Append to `backend/internal/store/game_store.go` (inside the store package, after `SetCrawlID`):

```go
// IncrementCurrentRound atomically bumps current_round by 1. Takes a DBExec
// so it can run inside the answer handler's transaction alongside the answer
// insert and the (last-round) status update.
func (s *GameStore) IncrementCurrentRound(ctx context.Context, db DBExec, id uuid.UUID) error {
    _, err := db.Exec(ctx,
        `UPDATE game_sessions SET current_round = current_round + 1, updated_at = NOW() WHERE id = $1`,
        id,
    )
    return err
}

// SetErrorMessage writes error_message. Used by the crawl goroutine's
// markSessionFailed helper; not transactional (a best-effort single-row update).
func (s *GameStore) SetErrorMessage(ctx context.Context, id uuid.UUID, msg string) error {
    _, err := s.pool.Exec(ctx,
        `UPDATE game_sessions SET error_message = $1, updated_at = NOW() WHERE id = $2`,
        msg, id,
    )
    return err
}
```

- [ ] **Step 8: Update existing `game_store_test.go` call site for `UpdateStatus`**

In `backend/internal/store/game_store_test.go:86`, change:
```go
err = gameStore.UpdateStatus(ctx, session.ID, models.GameStatusInProgress)
```
to:
```go
err = gameStore.UpdateStatus(ctx, pool, session.ID, models.GameStatusInProgress)
```

- [ ] **Step 9: Add tests for `IncrementCurrentRound` and `SetErrorMessage`**

Append to `backend/internal/store/game_store_test.go`:

```go
func TestGameStore_IncrementCurrentRound(t *testing.T) {
    pool := setupTestDB(t)
    gameStore := store.NewGameStore(pool)
    ctx := context.Background()

    shop := createTestShop(t, pool, "https://increment-test.com")
    session, err := gameStore.Create(ctx, shop.ID, "P1", models.GameModeComparison, 10)
    require.NoError(t, err)

    err = gameStore.IncrementCurrentRound(ctx, pool, session.ID)
    require.NoError(t, err)

    updated, err := gameStore.GetByID(ctx, session.ID)
    require.NoError(t, err)
    assert.Equal(t, 2, updated.CurrentRound)
}

func TestGameStore_SetErrorMessage(t *testing.T) {
    pool := setupTestDB(t)
    gameStore := store.NewGameStore(pool)
    ctx := context.Background()

    shop := createTestShop(t, pool, "https://errmsg-test.com")
    session, err := gameStore.Create(ctx, shop.ID, "P1", models.GameModeComparison, 10)
    require.NoError(t, err)

    err = gameStore.SetErrorMessage(ctx, session.ID, "crawl timed out")
    require.NoError(t, err)

    updated, err := gameStore.GetByID(ctx, session.ID)
    require.NoError(t, err)
    require.NotNil(t, updated.ErrorMessage)
    assert.Equal(t, "crawl timed out", *updated.ErrorMessage)
}
```

- [ ] **Step 10: Run the full store test package — everything must pass**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/store/ -v`

Expected: all tests (old + new) PASS. If the build fails because of non-store callers of `AnswerStore.Create` or `GameStore.UpdateStatus`, fix them to pass the pool — there should be none in `internal/` outside tests at this point, but run `cd backend && go build ./...` to be sure.

- [ ] **Step 11: Commit**

```bash
cd /home/jzy/projects/howmuchyousay
git add backend/internal/store/db_exec.go backend/internal/store/answer_store.go backend/internal/store/answer_store_test.go backend/internal/store/game_store.go backend/internal/store/game_store_test.go
git commit -m "$(cat <<'EOF'
feat(store): DBExec interface + transactional answer/game methods

Adds a shared DBExec interface (satisfied by both *pgxpool.Pool and
pgx.Tx) so AnswerStore.Create, GameStore.UpdateStatus, and the new
GameStore.IncrementCurrentRound can run inside a single transaction
from the upcoming answer handler. Also adds GameStore.SetErrorMessage
for the crawl failure path.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Server package scaffold + Gin dependency

**Files:**
- Create: `backend/internal/server/server.go`
- Modify: `backend/go.mod`, `backend/go.sum`

- [ ] **Step 1: Add Gin dependency**

Run: `cd backend && go get github.com/gin-gonic/gin`

Expected: `go get` succeeds, `go.mod` now contains `github.com/gin-gonic/gin` under `require`.

- [ ] **Step 2: Create the package stub with `Deps`, `Handler`, `New`, `Routes`**

Create `backend/internal/server/server.go`:

```go
// Package server hosts the HTTP API for the howmuchyousay game.
// See docs/superpowers/specs/2026-04-10-plan4-http-api-design.md.
package server

import (
    "log/slog"
    "math/rand/v2"

    "github.com/gin-gonic/gin"
    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/jzy/howmuchyousay/internal/config"
    "github.com/jzy/howmuchyousay/internal/store"
)

// Deps holds every dependency a handler may need. Injected via New().
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
    Rng      *rand.Rand
}

// Handler is the composition root for all HTTP handlers. Methods on *Handler
// are Gin handlers; they share the embedded Deps.
type Handler struct {
    Deps
}

// New constructs a Handler from a Deps value.
func New(d Deps) *Handler {
    return &Handler{Deps: d}
}

// Routes builds and returns the Gin engine with all routes registered.
func (h *Handler) Routes() *gin.Engine {
    r := gin.New()
    r.Use(gin.Logger(), gin.Recovery(), h.errorMiddleware())

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
```

At this point the file references `Crawler` (defined in Task 6), `h.errorMiddleware` (Task 4), and the handler methods (Tasks 8–13). That's fine — the build will not pass until those pieces land. **Do not attempt to build or commit yet.** We commit this scaffold after Task 4 (errors) and Task 6 (crawler interface) compile together. See Step 3.

- [ ] **Step 3: Skip build and commit until Task 4 lands**

Intentionally no build, no commit. This scaffolds the package namespace; it compiles only after Task 4 adds `errorMiddleware` and Task 6 adds the `Crawler` interface type. Proceed directly to Task 4.

---

## Task 4: Error middleware + APIError type

**Files:**
- Create: `backend/internal/server/errors.go`
- Create: `backend/internal/server/errors_test.go`

- [ ] **Step 1: Write the failing middleware test**

Create `backend/internal/server/errors_test.go`:

```go
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
```

- [ ] **Step 2: Run to verify fail**

Run: `cd backend && go test ./internal/server/ -run TestErrorMiddleware -v`

Expected: FAIL at compile — `undefined: ErrConflict`, `undefined: Handler.errorMiddleware`.

- [ ] **Step 3: Implement `errors.go`**

Create `backend/internal/server/errors.go`:

```go
package server

import (
    "errors"
    "net/http"

    "github.com/gin-gonic/gin"
)

// APIError is the structured error type that handlers emit via c.Error().
// The errorMiddleware renders it to a uniform JSON shape.
type APIError struct {
    Status  int
    Code    string
    Message string
    Details map[string]any
}

func (e *APIError) Error() string { return e.Message }

// With attaches a key/value pair that will be merged into the response body.
func (e *APIError) With(k string, v any) *APIError {
    if e.Details == nil {
        e.Details = map[string]any{}
    }
    e.Details[k] = v
    return e
}

func ErrBadRequest(msg string) *APIError {
    return &APIError{Status: http.StatusBadRequest, Code: "bad_request", Message: msg}
}

func ErrNotFound(what string) *APIError {
    return &APIError{Status: http.StatusNotFound, Code: "not_found", Message: what + " not found"}
}

func ErrConflict(code, msg string) *APIError {
    return &APIError{Status: http.StatusConflict, Code: code, Message: msg}
}

func ErrInternal() *APIError {
    return &APIError{Status: http.StatusInternalServerError, Code: "internal", Message: "internal server error"}
}

// errorMiddleware renders any APIError pushed onto c.Errors into a JSON body
// with shape {"error": ..., "code": ..., ...details}. Unknown errors render as
// 500 internal. No-error requests pass through untouched.
func (h *Handler) errorMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Next()
        if len(c.Errors) == 0 {
            return
        }
        last := c.Errors.Last().Err

        var apiErr *APIError
        if errors.As(last, &apiErr) {
            body := gin.H{"error": apiErr.Message, "code": apiErr.Code}
            for k, v := range apiErr.Details {
                body[k] = v
            }
            c.AbortWithStatusJSON(apiErr.Status, body)
            return
        }
        h.Logger.Error("unhandled handler error", "err", last)
        c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
            "error": "internal server error",
            "code":  "internal",
        })
    }
}
```

- [ ] **Step 4: Run the middleware tests — expect PASS**

The package still won't compile in full because `server.go` references `Crawler` and the handler methods that don't exist yet. Run only the error tests scoped to the current files — we need to temporarily satisfy the compiler. To keep this task self-contained and runnable, add empty stubs to `server.go` **temporarily** for the handlers and the crawler interface. Replace the existing `server.go` with the version below (this extends Task 3's scaffold with stubs that will be replaced in later tasks):

```go
package server

import (
    "context"
    "log/slog"
    "math/rand/v2"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/jzy/howmuchyousay/internal/config"
    "github.com/jzy/howmuchyousay/internal/crawler"
    "github.com/jzy/howmuchyousay/internal/store"
)

// Crawler is the subset of *crawler.Crawler that the server depends on.
// Defined in the consumer package so tests can inject a fake.
type Crawler interface {
    Run(ctx context.Context, cfg crawler.CrawlConfig, sessionID *uuid.UUID) (*crawler.CrawlResult, error)
}

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
    Rng      *rand.Rand
}

type Handler struct {
    Deps
}

func New(d Deps) *Handler { return &Handler{Deps: d} }

func (h *Handler) Routes() *gin.Engine {
    r := gin.New()
    r.Use(gin.Logger(), gin.Recovery(), h.errorMiddleware())
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

// ---- Handler stubs, replaced in later tasks ----

func (h *Handler) CreateGame(c *gin.Context)  { c.Status(501) }
func (h *Handler) GetSession(c *gin.Context)  { c.Status(501) }
func (h *Handler) GetRound(c *gin.Context)    { c.Status(501) }
func (h *Handler) PostAnswer(c *gin.Context)  { c.Status(501) }
func (h *Handler) GetResults(c *gin.Context)  { c.Status(501) }
```

Note: the `Crawler` interface is now defined here (it moves to `crawl_runner.go` in Task 6 with nothing changing but the file location). The handler stubs are placeholders each later task replaces exactly once.

- [ ] **Step 5: Run the error tests — expect PASS**

Run: `cd backend && go test ./internal/server/ -run TestErrorMiddleware -v`

Expected: PASS on all three middleware tests.

- [ ] **Step 6: Commit**

```bash
cd /home/jzy/projects/howmuchyousay
git add backend/internal/server/server.go backend/internal/server/errors.go backend/internal/server/errors_test.go backend/go.mod backend/go.sum
git commit -m "$(cat <<'EOF'
feat(server): scaffold package + APIError + error middleware

Introduces internal/server with a Deps struct, Handler, routes
skeleton (501 stubs), and the APIError/errorMiddleware plumbing
that renders typed errors to a uniform JSON shape. Adds Gin as a
dependency. Handler bodies are filled in by subsequent tasks.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Request/response DTOs

**Files:**
- Create: `backend/internal/server/dto.go`

No dedicated test file — DTOs are exercised by handler integration tests.

- [ ] **Step 1: Create `dto.go`**

Create `backend/internal/server/dto.go`:

```go
package server

import (
    "github.com/google/uuid"

    "github.com/jzy/howmuchyousay/internal/game"
    "github.com/jzy/howmuchyousay/internal/models"
)

// ---- Requests ----

type CreateGameRequest struct {
    Nick      string          `json:"nick"       binding:"required,min=1,max=32"`
    ShopURL   string          `json:"shop_url"   binding:"required,url"`
    GameMode  models.GameMode `json:"game_mode"  binding:"required,oneof=comparison guess"`
    SkipCrawl bool            `json:"skip_crawl"`
}

type SessionIDParams struct {
    SessionID uuid.UUID `uri:"session_id" binding:"required,uuid"`
}

type RoundParams struct {
    SessionID uuid.UUID `uri:"session_id" binding:"required,uuid"`
    Number    int       `uri:"number"     binding:"required,gt=0"`
}

type AnswerRequest struct {
    Answer string `json:"answer" binding:"required"`
}

// ---- Responses ----

type CreateGameResponse struct {
    SessionID uuid.UUID `json:"session_id"`
}

type SessionResponse struct {
    ID           uuid.UUID         `json:"id"`
    Status       models.GameStatus `json:"status"`
    GameMode     models.GameMode   `json:"game_mode"`
    RoundsTotal  int               `json:"rounds_total"`
    CurrentRound int               `json:"current_round"`
    ErrorMessage *string           `json:"error_message,omitempty"`
}

type RoundResponse struct {
    Number   int              `json:"number"`
    Type     models.RoundType `json:"type"`
    ProductA ProductDTO       `json:"product_a"`
    ProductB *ProductDTO      `json:"product_b,omitempty"`
}

// ProductDTO deliberately omits Price — players must not see prices before
// answering. Prices only appear in AnswerResponse.CorrectAnswer.
type ProductDTO struct {
    ID       uuid.UUID `json:"id"`
    Name     string    `json:"name"`
    ImageURL string    `json:"image_url"`
}

type AnswerResponse struct {
    IsCorrect     bool   `json:"is_correct"`
    Points        int    `json:"points"`
    CorrectAnswer string `json:"correct_answer"`
}

type ResultsResponse struct {
    SessionID uuid.UUID          `json:"session_id"`
    Rankings  []game.PlayerScore `json:"rankings"`
}

// productToDTO strips the price field from a product for client-facing payloads.
func productToDTO(p models.Product) ProductDTO {
    return ProductDTO{ID: p.ID, Name: p.Name, ImageURL: p.ImageURL}
}
```

- [ ] **Step 2: Build the server package to verify dto.go compiles**

Run: `cd backend && go build ./internal/server/`

Expected: success (no output).

- [ ] **Step 3: Commit**

```bash
cd /home/jzy/projects/howmuchyousay
git add backend/internal/server/dto.go
git commit -m "$(cat <<'EOF'
feat(server): request/response DTOs with binding tags

Adds the Gin-validated request structs (CreateGame, SessionID params,
Round params, Answer) and the client-facing response shapes. ProductDTO
deliberately omits Price so round fetches cannot leak the answer.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Move Crawler interface to crawl_runner.go + test helpers + fakeCrawler

**Files:**
- Create: `backend/internal/server/crawl_runner.go`
- Modify: `backend/internal/server/server.go` (remove `Crawler` type now that it lives in crawl_runner.go)
- Create: `backend/internal/server/testhelper_test.go`

- [ ] **Step 1: Create `crawl_runner.go` with the interface**

Create `backend/internal/server/crawl_runner.go`:

```go
package server

import (
    "context"

    "github.com/google/uuid"

    "github.com/jzy/howmuchyousay/internal/crawler"
)

// Crawler is the subset of *crawler.Crawler that the server depends on.
// Defined in the consumer package (Go idiom) so tests can inject a fake.
// The real *crawler.Crawler.Run matches this signature exactly.
type Crawler interface {
    Run(ctx context.Context, cfg crawler.CrawlConfig, sessionID *uuid.UUID) (*crawler.CrawlResult, error)
}

// runCrawlForSession and markSessionFailed are filled in by Task 9.
```

- [ ] **Step 2: Remove the duplicate `Crawler` type + `context` import from `server.go`**

Edit `backend/internal/server/server.go`:

- Remove the `Crawler` interface declaration block.
- Remove the `"context"` import (no longer needed in this file).
- Remove `"github.com/google/uuid"` import (no longer needed).
- Remove `"github.com/jzy/howmuchyousay/internal/crawler"` import (no longer needed).

After editing, `server.go` should have imports: `"log/slog"`, `"math/rand/v2"`, `"github.com/gin-gonic/gin"`, `"github.com/jackc/pgx/v5/pgxpool"`, `"github.com/jzy/howmuchyousay/internal/config"`, `"github.com/jzy/howmuchyousay/internal/store"`.

- [ ] **Step 3: Verify build**

Run: `cd backend && go build ./internal/server/`

Expected: success.

- [ ] **Step 4: Create `testhelper_test.go` with setupTestDB, cleanupTestDB, setupTestHandler, fakeCrawler, seedShopWithProducts**

Create `backend/internal/server/testhelper_test.go`:

```go
package server

import (
    "context"
    "fmt"
    "io"
    "log/slog"
    "math/rand/v2"
    "os"
    "sync/atomic"
    "testing"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/stretchr/testify/require"

    "github.com/jzy/howmuchyousay/internal/config"
    "github.com/jzy/howmuchyousay/internal/crawler"
    "github.com/jzy/howmuchyousay/internal/models"
    "github.com/jzy/howmuchyousay/internal/store"
)

// setupTestDB connects to the shared test database, runs migrations, and
// registers cleanup that truncates every game-related table.
func setupTestDB(t *testing.T) *pgxpool.Pool {
    t.Helper()
    dbURL := os.Getenv("TEST_DATABASE_URL")
    if dbURL == "" {
        dbURL = "postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable"
    }
    require.NoError(t, store.RunMigrations(dbURL, "../../migrations"))

    pool, err := store.ConnectDB(dbURL)
    require.NoError(t, err)

    t.Cleanup(func() {
        cleanupTestDB(t, pool)
        pool.Close()
    })
    return pool
}

func cleanupTestDB(t *testing.T, pool *pgxpool.Pool) {
    t.Helper()
    _, _ = pool.Exec(context.Background(), "UPDATE game_sessions SET crawl_id = NULL")
    for _, tbl := range []string{"answers", "rounds", "players", "products", "crawls", "game_sessions", "shops"} {
        if _, err := pool.Exec(context.Background(), "DELETE FROM "+tbl); err != nil {
            t.Logf("cleanup: %s: %v", tbl, err)
        }
    }
}

// setupTestHandler returns a Handler wired against the real test DB and a
// controllable fakeCrawler. The returned pool is the same one the Handler holds.
func setupTestHandler(t *testing.T) (*Handler, *pgxpool.Pool, *fakeCrawler) {
    t.Helper()
    pool := setupTestDB(t)
    fake := &fakeCrawler{}
    h := New(Deps{
        Pool:     pool,
        Sessions: store.NewGameStore(pool),
        Shops:    store.NewShopStore(pool),
        Players:  store.NewPlayerStore(pool),
        Products: store.NewProductStore(pool),
        Rounds:   store.NewRoundStore(pool),
        Answers:  store.NewAnswerStore(pool),
        Crawler:  fake,
        Config:   &config.Config{CrawlTimeout: 10, FirecrawlMaxScrapes: 5},
        Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
        Rng:      rand.New(rand.NewPCG(42, 0)),
    })
    return h, pool, fake
}

// fakeCrawler lets tests control what Crawler.Run does. If `behavior` is nil,
// Run returns an error. If `done` is non-nil, it is closed once behavior returns
// (for tests that need to wait on the async goroutine without sleeping).
type fakeCrawler struct {
    behavior func(ctx context.Context, cfg crawler.CrawlConfig, sessionID *uuid.UUID) (*crawler.CrawlResult, error)
    done     chan struct{}
    called   atomic.Int32
}

func (f *fakeCrawler) Run(ctx context.Context, cfg crawler.CrawlConfig, sessionID *uuid.UUID) (*crawler.CrawlResult, error) {
    f.called.Add(1)
    defer func() {
        if f.done != nil {
            close(f.done)
        }
    }()
    if f.behavior == nil {
        return nil, fmt.Errorf("fakeCrawler: behavior not set")
    }
    return f.behavior(ctx, cfg, sessionID)
}

// seedShopWithProducts inserts a shop, a crawl row linked to that shop, and N
// products priced at 100, 200, 300, ... so comparison pairs have comfortable
// price deltas. Returns the shop and the created products.
func seedShopWithProducts(t *testing.T, pool *pgxpool.Pool, shopURL string, n int) (*models.Shop, []models.Product) {
    t.Helper()
    ctx := context.Background()

    shop, err := store.NewShopStore(pool).Create(ctx, shopURL)
    require.NoError(t, err)

    crawl, err := store.NewCrawlStore(pool).Create(ctx, shop.ID, nil, "/tmp/test.log")
    require.NoError(t, err)

    productStore := store.NewProductStore(pool)
    products := make([]models.Product, 0, n)
    for i := 0; i < n; i++ {
        p, err := productStore.Create(ctx, shop.ID, crawl.ID,
            fmt.Sprintf("Product %d", i), float64((i+1)*100),
            fmt.Sprintf("https://img/%d.jpg", i), fmt.Sprintf("https://src/%d", i))
        require.NoError(t, err)
        products = append(products, *p)
    }
    return shop, products
}
```

- [ ] **Step 5: Build the test package**

Run: `cd backend && go test ./internal/server/ -run NoSuchTestName -v`

Expected: "no tests to run" — compile succeeds, no tests match the name. If the compile fails, fix before proceeding.

- [ ] **Step 6: Commit**

```bash
cd /home/jzy/projects/howmuchyousay
git add backend/internal/server/server.go backend/internal/server/crawl_runner.go backend/internal/server/testhelper_test.go
git commit -m "$(cat <<'EOF'
feat(server): Crawler interface + test helpers + fakeCrawler

Moves the Crawler interface out of server.go into crawl_runner.go
(its permanent home) and adds the integration test harness:
setupTestDB, cleanupTestDB, setupTestHandler, a controllable
fakeCrawler with a done channel for sleep-free goroutine sync,
and a seedShopWithProducts helper.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: rounds.go — generateAndPersistRounds helper

**Files:**
- Create: `backend/internal/server/rounds.go`

This helper is used by both the synchronous `skip_crawl=true` path (Task 8) and the async crawl goroutine (Task 9). Extracting it first avoids duplication.

- [ ] **Step 1: Create `rounds.go`**

Create `backend/internal/server/rounds.go`:

```go
package server

import (
    "context"
    "fmt"

    "github.com/google/uuid"

    "github.com/jzy/howmuchyousay/internal/game"
    "github.com/jzy/howmuchyousay/internal/models"
    "github.com/jzy/howmuchyousay/internal/store"
)

// defaultRoundsTotal is the single-player game length for Plan 4.
const defaultRoundsTotal = 10

// generateRounds picks the appropriate generator based on the game mode and
// returns the round definitions. It does NOT touch the database.
func (h *Handler) generateRounds(mode models.GameMode, products []models.Product, count int) ([]game.RoundDef, error) {
    switch mode {
    case models.GameModeComparison:
        return game.GenerateComparisonRounds(products, count, h.Rng)
    case models.GameModeGuess:
        return game.GenerateGuessRounds(products, count, h.Rng)
    default:
        return nil, fmt.Errorf("unknown game mode: %s", mode)
    }
}

// persistRounds inserts round definitions into the rounds table via the
// given DBExec (pool or tx). The round ordering is preserved.
func persistRounds(ctx context.Context, db store.DBExec, sessionID uuid.UUID, defs []game.RoundDef) error {
    rs := store.NewRoundStoreForExec(db)
    for _, d := range defs {
        var productBID *uuid.UUID
        if d.ProductB != nil {
            id := d.ProductB.ID
            productBID = &id
        }
        if _, err := rs.Create(ctx, sessionID, d.RoundNumber, d.RoundType, d.ProductA.ID, productBID, d.CorrectAnswer, d.DifficultyScore); err != nil {
            return fmt.Errorf("persist round %d: %w", d.RoundNumber, err)
        }
    }
    return nil
}
```

This uses a `store.NewRoundStoreForExec(db)` constructor that doesn't exist yet — that's fine, we add it next.

- [ ] **Step 2: Add `NewRoundStoreForExec` to round_store.go**

Edit `backend/internal/store/round_store.go`. Add a second constructor and a pointer to keep both styles working:

```go
// NewRoundStoreForExec returns a RoundStore that uses the given DBExec for
// writes. Used when rounds must be inserted inside an externally-managed
// transaction (e.g. by the CreateGame skip_crawl=true path in internal/server).
func NewRoundStoreForExec(db DBExec) *RoundStore {
    return &RoundStore{pool: nil, exec: db}
}
```

And simplify the `RoundStore` struct to hold a single `DBExec` field that satisfies every method:

```go
type RoundStore struct {
    exec DBExec
}

func NewRoundStore(pool *pgxpool.Pool) *RoundStore {
    return &RoundStore{exec: pool}
}
```

Then change every `s.pool.QueryRow(...)` / `s.pool.Query(...)` / `s.pool.Exec(...)` reference in `round_store.go` to `s.exec.QueryRow(...)` / etc. There are three methods to update: `Create`, `GetBySessionID`, `GetBySessionAndNumber`. The `pgxpool` import stays because `NewRoundStore` still takes `*pgxpool.Pool`.

`NewRoundStoreForExec(db DBExec)` returns `&RoundStore{exec: db}` — a tx-bound store whose reads and writes both run on the transaction.

- [ ] **Step 3: Run store tests — expect still PASS**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/store/ -v`

Expected: all PASS. `NewRoundStore(pool)` now also sets `exec: pool`, so all existing callers keep working.

- [ ] **Step 4: Build the server package**

Run: `cd backend && go build ./internal/server/`

Expected: success.

- [ ] **Step 5: Commit**

```bash
cd /home/jzy/projects/howmuchyousay
git add backend/internal/server/rounds.go backend/internal/store/round_store.go
git commit -m "$(cat <<'EOF'
feat(server): generateRounds + persistRounds helpers

Extracts the round generation/persistence path used by both the
synchronous skip_crawl=true CreateGame and the async crawl
goroutine into rounds.go. Adds NewRoundStoreForExec so rounds can
be inserted inside a caller-managed transaction.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: CreateGame handler — `skip_crawl=true` path

**Files:**
- Create: `backend/internal/server/game_handlers.go` (CreateGame only for now)
- Delete the `CreateGame` stub from `server.go` at the same time (replace with real method body)
- Create: `backend/internal/server/create_game_test.go`

- [ ] **Step 1: Write the failing integration test**

Create `backend/internal/server/create_game_test.go`:

```go
package server

import (
    "bytes"
    "context"
    "encoding/json"
    "io"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/google/uuid"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "github.com/jzy/howmuchyousay/internal/models"
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
    w := postJSON(t, h, "/api/game", map[string]any{"nick": "me"}) // missing shop_url, game_mode
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
    _, _ = seedShopWithProducts(t, pool, "https://skipcrawl-success.com", 20)

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
    _, _ = seedShopWithProducts(t, pool, "https://skipcrawl-few.com", 5)

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
```

- [ ] **Step 2: Run the test — expect FAIL (stub returns 501)**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/server/ -run TestCreateGame -v`

Expected: FAIL — tests get 501 from the stub.

- [ ] **Step 3: Implement `CreateGame` in `game_handlers.go`**

Create `backend/internal/server/game_handlers.go`:

```go
package server

import (
    "context"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"

    "github.com/jzy/howmuchyousay/internal/models"
)

// CreateGame handles POST /api/game.
func (h *Handler) CreateGame(c *gin.Context) {
    var req CreateGameRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.Error(ErrBadRequest(err.Error()))
        return
    }
    ctx := c.Request.Context()

    // Find-or-create the shop.
    shop, err := h.Shops.GetByURL(ctx, req.ShopURL)
    if err != nil {
        h.Logger.Error("shops.GetByURL", "err", err)
        c.Error(ErrInternal())
        return
    }
    if shop == nil {
        shop, err = h.Shops.Create(ctx, req.ShopURL)
        if err != nil {
            h.Logger.Error("shops.Create", "err", err)
            c.Error(ErrInternal())
            return
        }
    }

    if req.SkipCrawl {
        h.createGameSkipCrawl(c, req, shop.ID)
        return
    }

    // Async-crawl path is implemented in Task 9.
    c.Error(ErrInternal())
}

// createGameSkipCrawl is the synchronous path used when the caller already
// has products for the shop in the DB. All DB mutations happen inside a
// single transaction so a failure at any step rolls back cleanly.
func (h *Handler) createGameSkipCrawl(c *gin.Context, req CreateGameRequest, shopID uuid.UUID) {
    ctx := c.Request.Context()

    count, err := h.Products.CountByShopID(ctx, shopID)
    if err != nil {
        h.Logger.Error("products.CountByShopID", "err", err)
        c.Error(ErrInternal())
        return
    }
    if count < 20 {
        c.Error(ErrConflict("not_enough_products", "shop has fewer than 20 products").With("count", count))
        return
    }

    // Fetch a sized random sample, generate rounds in memory — no writes yet.
    products, err := h.Products.GetRandomByShopID(ctx, shopID, 2*defaultRoundsTotal)
    if err != nil {
        h.Logger.Error("products.GetRandomByShopID", "err", err)
        c.Error(ErrInternal())
        return
    }
    defs, err := h.generateRounds(req.GameMode, products, defaultRoundsTotal)
    if err != nil {
        c.Error(ErrConflict("round_generation_failed", err.Error()))
        return
    }

    tx, err := h.Pool.BeginTx(ctx, pgx.TxOptions{})
    if err != nil {
        h.Logger.Error("begin tx", "err", err)
        c.Error(ErrInternal())
        return
    }
    committed := false
    defer func() {
        if !committed {
            _ = tx.Rollback(context.Background())
        }
    }()

    // Insert session row (status=in_progress) — we reuse the store method,
    // which runs on the pool; inside-tx semantics are not needed here because
    // we roll back by deleting what we inserted (see below). To keep it
    // truly atomic, we instead issue the INSERT via tx directly.
    var sessionID uuid.UUID
    err = tx.QueryRow(ctx,
        `INSERT INTO game_sessions (shop_id, host_nick, game_mode, rounds_total, status)
         VALUES ($1, $2, $3, $4, 'in_progress')
         RETURNING id`,
        shopID, req.Nick, req.GameMode, defaultRoundsTotal,
    ).Scan(&sessionID)
    if err != nil {
        h.Logger.Error("insert game_sessions", "err", err)
        c.Error(ErrInternal())
        return
    }

    _, err = tx.Exec(ctx,
        `INSERT INTO players (session_id, nick, is_host) VALUES ($1, $2, true)`,
        sessionID, req.Nick,
    )
    if err != nil {
        h.Logger.Error("insert players", "err", err)
        c.Error(ErrInternal())
        return
    }

    if err := persistRounds(ctx, tx, sessionID, defs); err != nil {
        h.Logger.Error("persistRounds", "err", err)
        c.Error(ErrInternal())
        return
    }

    if err := tx.Commit(ctx); err != nil {
        h.Logger.Error("commit", "err", err)
        c.Error(ErrInternal())
        return
    }
    committed = true

    c.JSON(http.StatusCreated, CreateGameResponse{SessionID: sessionID})
}
```

Also: remove the `CreateGame` stub line from `server.go`:

```go
func (h *Handler) CreateGame(c *gin.Context) { c.Status(501) }
```

- [ ] **Step 4: Run the test — expect PASS on validation + skip_crawl_success + not_enough_products**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/server/ -run TestCreateGame -v`

Expected: `TestCreateGame_Validation_*`, `TestCreateGame_SkipCrawl_Success`, and `TestCreateGame_SkipCrawl_NotEnoughProducts` all PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/jzy/projects/howmuchyousay
git add backend/internal/server/server.go backend/internal/server/game_handlers.go backend/internal/server/create_game_test.go
git commit -m "$(cat <<'EOF'
feat(server): POST /api/game skip_crawl=true path

Implements the synchronous CreateGame path: find-or-create shop,
verify >= 20 products, generate rounds in memory, then insert
session+player+rounds inside a single pgx transaction so any
failure rolls back cleanly. Covers validation, success, and the
not_enough_products conflict.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: CreateGame `skip_crawl=false` path + runCrawlForSession goroutine

**Files:**
- Modify: `backend/internal/server/game_handlers.go` (CreateGame wires async path)
- Modify: `backend/internal/server/crawl_runner.go` (runCrawlForSession, markSessionFailed)
- Modify: `backend/internal/server/create_game_test.go` (add async tests)

- [ ] **Step 1: Write the failing tests for the async path**

Append to `backend/internal/server/create_game_test.go`:

```go
func TestCreateGame_WithCrawl_Success(t *testing.T) {
    h, pool, fake := setupTestHandler(t)
    ctx := context.Background()

    // Seed shop + crawl + products that the fake will "produce".
    shop, products := seedShopWithProducts(t, pool, "https://withcrawl-success.com", 20)

    fake.done = make(chan struct{})
    fake.behavior = func(ctx context.Context, cfg crawler.CrawlConfig, sessionID *uuid.UUID) (*crawler.CrawlResult, error) {
        // Create a new crawl row to simulate a real run and return its ID.
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

    // Wait for the fake to finish — no sleeps.
    select {
    case <-fake.done:
    case <-time.After(5 * time.Second):
        t.Fatal("crawl goroutine did not complete")
    }
    // Give the goroutine's post-crawl DB writes a tick to land. Poll, don't sleep.
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

// waitForStatus polls GetByID until the session reaches the target status or
// the timeout fires. Used to synchronize with the detached crawl goroutine
// without relying on time.Sleep.
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
```

Add these imports at the top of `create_game_test.go`:
```go
"errors"
"time"

"github.com/jzy/howmuchyousay/internal/crawler"
"github.com/jzy/howmuchyousay/internal/store"
```

- [ ] **Step 2: Run — expect FAIL**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/server/ -run "TestCreateGame_WithCrawl" -v`

Expected: FAIL — async path currently returns 500 via the placeholder `ErrInternal` branch.

- [ ] **Step 3: Fill in `crawl_runner.go`**

Replace the contents of `backend/internal/server/crawl_runner.go` with:

```go
package server

import (
    "context"
    "time"

    "github.com/google/uuid"

    "github.com/jzy/howmuchyousay/internal/crawler"
    "github.com/jzy/howmuchyousay/internal/models"
)

// Crawler is the subset of *crawler.Crawler that the server depends on.
type Crawler interface {
    Run(ctx context.Context, cfg crawler.CrawlConfig, sessionID *uuid.UUID) (*crawler.CrawlResult, error)
}

// runCrawlForSession executes a crawl for the given session in a detached
// goroutine. It uses context.Background()-derived contexts exclusively
// because c.Request.Context() was cancelled the moment the HTTP handler
// returned — using it here would kill the crawl immediately.
//
// On success: SetCrawlID, load products, generate + persist rounds,
// transition status crawling → ready.
// On any failure: transition status crawling → failed with error_message set.
func (h *Handler) runCrawlForSession(sessionID, shopID uuid.UUID, shopURL string, mode models.GameMode) {
    ctx, cancel := context.WithTimeout(
        context.Background(),
        time.Duration(h.Config.CrawlTimeout)*time.Second,
    )
    defer cancel()

    cfg := crawler.DefaultCrawlConfig(shopURL)
    cfg.MaxScrapes = h.Config.FirecrawlMaxScrapes
    cfg.Timeout = time.Duration(h.Config.CrawlTimeout) * time.Second

    result, err := h.Crawler.Run(ctx, cfg, &sessionID)
    if err != nil {
        h.markSessionFailed(sessionID, err.Error())
        return
    }

    bg := context.Background()

    if err := h.Sessions.SetCrawlID(bg, sessionID, result.CrawlID); err != nil {
        h.markSessionFailed(sessionID, "set crawl id: "+err.Error())
        return
    }

    products, err := h.Products.GetByShopID(bg, shopID)
    if err != nil {
        h.markSessionFailed(sessionID, "load products: "+err.Error())
        return
    }
    if len(products) < 20 {
        h.markSessionFailed(sessionID, "not enough products after crawl")
        return
    }

    defs, err := h.generateRounds(mode, products, defaultRoundsTotal)
    if err != nil {
        h.markSessionFailed(sessionID, "round generation: "+err.Error())
        return
    }
    if err := persistRounds(bg, h.Pool, sessionID, defs); err != nil {
        h.markSessionFailed(sessionID, "persist rounds: "+err.Error())
        return
    }

    if err := h.Sessions.UpdateStatus(bg, h.Pool, sessionID, models.GameStatusReady); err != nil {
        // Rounds already persisted, but the status didn't flip — surface
        // via the failure path rather than leaving it wedged in "crawling".
        h.markSessionFailed(sessionID, "mark ready: "+err.Error())
        return
    }
}

// markSessionFailed is a best-effort pair of writes: set error_message, then
// flip status to failed. Both calls log on error because there's nothing
// sensible to do if the DB is unreachable — the session is already broken.
func (h *Handler) markSessionFailed(sessionID uuid.UUID, msg string) {
    bg := context.Background()
    if err := h.Sessions.SetErrorMessage(bg, sessionID, msg); err != nil {
        h.Logger.Error("set error message", "err", err, "session_id", sessionID)
    }
    if err := h.Sessions.UpdateStatus(bg, h.Pool, sessionID, models.GameStatusFailed); err != nil {
        h.Logger.Error("mark session failed", "err", err, "session_id", sessionID)
    }
}
```

- [ ] **Step 4: Wire the async branch in `CreateGame`**

In `backend/internal/server/game_handlers.go`, replace the `// Async-crawl path...` block with a real implementation:

```go
    // Async-crawl path: create session (status=crawling), create host player,
    // spawn goroutine, return 201 immediately.
    session, err := h.Sessions.Create(ctx, shop.ID, req.Nick, req.GameMode, defaultRoundsTotal)
    if err != nil {
        h.Logger.Error("sessions.Create", "err", err)
        c.Error(ErrInternal())
        return
    }
    if _, err := h.Players.Create(ctx, session.ID, req.Nick, true); err != nil {
        h.Logger.Error("players.Create", "err", err)
        c.Error(ErrInternal())
        return
    }

    go h.runCrawlForSession(session.ID, shop.ID, req.ShopURL, req.GameMode)

    c.JSON(http.StatusCreated, CreateGameResponse{SessionID: session.ID})
```

No import cleanup needed — `"errors"` is not imported in Task 8's version of `game_handlers.go`, so there is nothing to remove here.

- [ ] **Step 5: Run the new tests — expect PASS**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/server/ -run TestCreateGame -v`

Expected: all CreateGame tests (sync + async success + async failure) PASS.

- [ ] **Step 6: Commit**

```bash
cd /home/jzy/projects/howmuchyousay
git add backend/internal/server/crawl_runner.go backend/internal/server/game_handlers.go backend/internal/server/create_game_test.go
git commit -m "$(cat <<'EOF'
feat(server): POST /api/game async crawl path

Wires the fire-and-forget crawl goroutine: create session in
crawling status, return 201 immediately, then runCrawlForSession
loads products, generates rounds, and transitions to ready — or
to failed with error_message on any failure. Uses
context.Background() exclusively so the detached goroutine is
not cancelled when the HTTP request returns.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: GetSession handler

**Files:**
- Modify: `backend/internal/server/game_handlers.go` (replace GetSession stub)
- Modify: `backend/internal/server/server.go` (remove GetSession stub)
- Create: `backend/internal/server/get_session_test.go`

- [ ] **Step 1: Write the failing test**

Create `backend/internal/server/get_session_test.go`:

```go
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
```

- [ ] **Step 2: Run — expect FAIL**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/server/ -run TestGetSession -v`

Expected: FAIL — GetSession still returns 501.

- [ ] **Step 3: Implement `GetSession`**

Append to `backend/internal/server/game_handlers.go`:

```go
// GetSession handles GET /api/game/:session_id.
func (h *Handler) GetSession(c *gin.Context) {
    var params SessionIDParams
    if err := c.ShouldBindUri(&params); err != nil {
        c.Error(ErrBadRequest(err.Error()))
        return
    }

    session, err := h.Sessions.GetByID(c.Request.Context(), params.SessionID)
    if err != nil {
        h.Logger.Error("sessions.GetByID", "err", err)
        c.Error(ErrInternal())
        return
    }
    if session == nil {
        c.Error(ErrNotFound("session"))
        return
    }

    c.JSON(http.StatusOK, SessionResponse{
        ID:           session.ID,
        Status:       session.Status,
        GameMode:     session.GameMode,
        RoundsTotal:  session.RoundsTotal,
        CurrentRound: session.CurrentRound,
        ErrorMessage: session.ErrorMessage,
    })
}
```

Remove the `GetSession` stub from `server.go`.

- [ ] **Step 4: Run — expect PASS**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/server/ -run TestGetSession -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/jzy/projects/howmuchyousay
git add backend/internal/server/game_handlers.go backend/internal/server/server.go backend/internal/server/get_session_test.go
git commit -m "$(cat <<'EOF'
feat(server): GET /api/game/:session_id

Returns the session's current state: status, mode, rounds_total,
current_round, and error_message (populated for failed sessions).
This endpoint is the single-player polling surface for observing
the crawling → ready → in_progress → finished transitions.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: GetRound handler

**Files:**
- Modify: `backend/internal/server/game_handlers.go` (replace GetRound stub)
- Modify: `backend/internal/server/server.go` (remove GetRound stub)
- Create: `backend/internal/server/get_round_test.go`

- [ ] **Step 1: Write the failing tests**

Create `backend/internal/server/get_round_test.go`:

```go
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

// createSkipCrawlSession is a shortcut used by get_round / post_answer / get_results tests.
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

    // Hard guarantee: the raw JSON must never contain the word "price".
    w2 := getJSON(t, h, "/api/game/"+sid.String()+"/round/1")
    assert.False(t, strings.Contains(strings.ToLower(w2.Body.String()), "price"),
        "round payload leaked a price field: %s", w2.Body.String())
}

func TestGetRound_FinishedSession_Rejected(t *testing.T) {
    h, _, _ := setupTestHandler(t)
    sid := createSkipCrawlSession(t, h, "https://getround-finished.com", "comparison")

    // Force-finish the session via the store directly.
    require.NoError(t, h.Sessions.UpdateStatus(context.Background(), h.Pool, sid, models.GameStatusFinished))

    w := getJSON(t, h, "/api/game/"+sid.String()+"/round/1")
    assert.Equal(t, http.StatusConflict, w.Code)
}

func TestGetRound_ReadyToInProgressTransition(t *testing.T) {
    h, _, _ := setupTestHandler(t)
    sid := createSkipCrawlSession(t, h, "https://getround-ready.com", "comparison")

    // Rewind the session to "ready" to exercise the implicit transition.
    require.NoError(t, h.Sessions.UpdateStatus(context.Background(), h.Pool, sid, models.GameStatusReady))

    w := getJSON(t, h, "/api/game/"+sid.String()+"/round/1")
    require.Equal(t, http.StatusOK, w.Code)

    session, err := h.Sessions.GetByID(context.Background(), sid)
    require.NoError(t, err)
    assert.Equal(t, models.GameStatusInProgress, session.Status)
}

// number-not-integer is rejected at the binding layer:
func TestGetRound_InvalidNumber(t *testing.T) {
    h, _, _ := setupTestHandler(t)
    sid := createSkipCrawlSession(t, h, "https://getround-badnum.com", "comparison")

    w := getJSON(t, h, "/api/game/"+sid.String()+"/round/zero")
    assert.Equal(t, http.StatusBadRequest, w.Code)
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/server/ -run TestGetRound -v`

Expected: FAIL — GetRound returns 501.

- [ ] **Step 3: Implement `GetRound`**

Append to `backend/internal/server/game_handlers.go`:

```go
// GetRound handles GET /api/game/:session_id/round/:number.
func (h *Handler) GetRound(c *gin.Context) {
    var params RoundParams
    if err := c.ShouldBindUri(&params); err != nil {
        c.Error(ErrBadRequest(err.Error()))
        return
    }
    ctx := c.Request.Context()

    session, err := h.Sessions.GetByID(ctx, params.SessionID)
    if err != nil {
        h.Logger.Error("sessions.GetByID", "err", err)
        c.Error(ErrInternal())
        return
    }
    if session == nil {
        c.Error(ErrNotFound("session"))
        return
    }

    if session.Status != models.GameStatusReady && session.Status != models.GameStatusInProgress {
        c.Error(ErrConflict("session_not_playable", "session is not playable").
            With("status", string(session.Status)))
        return
    }
    if params.Number != session.CurrentRound {
        c.Error(ErrConflict("not_current_round", "not current round").
            With("current_round", session.CurrentRound))
        return
    }

    round, err := h.Rounds.GetBySessionAndNumber(ctx, params.SessionID, params.Number)
    if err != nil {
        h.Logger.Error("rounds.GetBySessionAndNumber", "err", err)
        c.Error(ErrInternal())
        return
    }
    if round == nil {
        c.Error(ErrNotFound("round"))
        return
    }

    productA, err := h.Products.GetByID(ctx, round.ProductAID)
    if err != nil || productA == nil {
        h.Logger.Error("load product A", "err", err)
        c.Error(ErrInternal())
        return
    }

    resp := RoundResponse{
        Number:   round.RoundNumber,
        Type:     round.RoundType,
        ProductA: productToDTO(*productA),
    }
    if round.ProductBID != nil {
        productB, err := h.Products.GetByID(ctx, *round.ProductBID)
        if err != nil || productB == nil {
            h.Logger.Error("load product B", "err", err)
            c.Error(ErrInternal())
            return
        }
        dto := productToDTO(*productB)
        resp.ProductB = &dto
    }

    // Implicit ready → in_progress transition on first successful round fetch.
    if session.Status == models.GameStatusReady {
        if err := h.Sessions.UpdateStatus(ctx, h.Pool, session.ID, models.GameStatusInProgress); err != nil {
            h.Logger.Error("flip ready→in_progress", "err", err)
            c.Error(ErrInternal())
            return
        }
    }

    c.JSON(http.StatusOK, resp)
}
```

This references `ProductStore.GetByID`, which doesn't exist. Add it — see next step. (For Plan 4 the round's products are guaranteed to exist via referential integrity on round rows, so a nil return here is a 500-worthy invariant violation, not a 404.)

- [ ] **Step 4: Add `ProductStore.GetByID`**

Append to `backend/internal/store/product_store.go`:

```go
import (
    "context"
    "errors"
    // ... existing imports ...
    "github.com/jackc/pgx/v5"
)

// (add alongside existing methods)

func (s *ProductStore) GetByID(ctx context.Context, id uuid.UUID) (*models.Product, error) {
    p := &models.Product{}
    err := s.pool.QueryRow(ctx,
        `SELECT id, shop_id, crawl_id, name, price, image_url, source_url, created_at
         FROM products WHERE id = $1`,
        id,
    ).Scan(&p.ID, &p.ShopID, &p.CrawlID, &p.Name, &p.Price, &p.ImageURL, &p.SourceURL, &p.CreatedAt)
    if errors.Is(err, pgx.ErrNoRows) {
        return nil, nil
    }
    if err != nil {
        return nil, err
    }
    return p, nil
}
```

Merge the import block cleanly (add `"errors"` and `"github.com/jackc/pgx/v5"` if absent).

- [ ] **Step 5: Remove `GetRound` stub from `server.go`**

Delete the line:
```go
func (h *Handler) GetRound(c *gin.Context) { c.Status(501) }
```

- [ ] **Step 6: Run — expect PASS**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/server/ -run TestGetRound -v`

Expected: all GetRound tests PASS.

- [ ] **Step 7: Commit**

```bash
cd /home/jzy/projects/howmuchyousay
git add backend/internal/server/game_handlers.go backend/internal/server/server.go backend/internal/server/get_round_test.go backend/internal/store/product_store.go
git commit -m "$(cat <<'EOF'
feat(server): GET /api/game/:session_id/round/:number

Round fetch with full state gating: session must be ready or
in_progress, requested number must match session.current_round.
First successful round fetch implicitly flips ready → in_progress.
ProductDTO omits price so the endpoint cannot leak the answer.

Also adds ProductStore.GetByID (needed to load round products by
id without filtering the full shop list).

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: PostAnswer handler (transactional advance + duplicate handling)

**Files:**
- Modify: `backend/internal/server/game_handlers.go`
- Modify: `backend/internal/server/server.go`
- Create: `backend/internal/server/post_answer_test.go`

- [ ] **Step 1: Write the failing tests**

Create `backend/internal/server/post_answer_test.go`:

```go
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

// loadCorrectAnswer pulls the correct answer for a specific round out of the DB.
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

    // First answer.
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
```

Add imports at the top: `"fmt"`, `"github.com/google/uuid"`.

- [ ] **Step 2: Run — expect FAIL**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/server/ -run TestPostAnswer -v`

Expected: FAIL — PostAnswer returns 501.

- [ ] **Step 3: Implement `PostAnswer`**

Append to `backend/internal/server/game_handlers.go`:

```go
// PostAnswer handles POST /api/game/:session_id/round/:number/answer.
func (h *Handler) PostAnswer(c *gin.Context) {
    var params RoundParams
    if err := c.ShouldBindUri(&params); err != nil {
        c.Error(ErrBadRequest(err.Error()))
        return
    }
    var req AnswerRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.Error(ErrBadRequest(err.Error()))
        return
    }
    ctx := c.Request.Context()

    session, err := h.Sessions.GetByID(ctx, params.SessionID)
    if err != nil {
        h.Logger.Error("sessions.GetByID", "err", err)
        c.Error(ErrInternal())
        return
    }
    if session == nil {
        c.Error(ErrNotFound("session"))
        return
    }
    if session.Status != models.GameStatusInProgress {
        c.Error(ErrConflict("session_not_in_progress", "session is not in progress").
            With("status", string(session.Status)))
        return
    }
    if params.Number != session.CurrentRound {
        c.Error(ErrConflict("not_current_round", "not current round").
            With("current_round", session.CurrentRound))
        return
    }

    round, err := h.Rounds.GetBySessionAndNumber(ctx, session.ID, params.Number)
    if err != nil {
        h.Logger.Error("rounds.GetBySessionAndNumber", "err", err)
        c.Error(ErrInternal())
        return
    }
    if round == nil {
        c.Error(ErrNotFound("round"))
        return
    }

    productA, err := h.Products.GetByID(ctx, round.ProductAID)
    if err != nil || productA == nil {
        h.Logger.Error("load product A", "err", err)
        c.Error(ErrInternal())
        return
    }
    var productB *models.Product
    if round.ProductBID != nil {
        productB, err = h.Products.GetByID(ctx, *round.ProductBID)
        if err != nil || productB == nil {
            h.Logger.Error("load product B", "err", err)
            c.Error(ErrInternal())
            return
        }
    }

    // Host player lookup — Plan 4 is single-player, so there is exactly one
    // player per session and it's the host.
    players, err := h.Players.GetBySessionID(ctx, session.ID)
    if err != nil || len(players) == 0 {
        h.Logger.Error("players.GetBySessionID", "err", err)
        c.Error(ErrInternal())
        return
    }
    playerID := players[0].ID

    // Answer-format validation + scoring.
    var isCorrect bool
    var points int
    var correctAnswerForResponse string

    switch round.RoundType {
    case models.RoundTypeComparison:
        if req.Answer != "a" && req.Answer != "b" {
            c.Error(ErrBadRequest("comparison answer must be 'a' or 'b'"))
            return
        }
        if productB == nil {
            c.Error(ErrInternal())
            return
        }
        isCorrect, points = game.EvalComparison(req.Answer, round.CorrectAnswer, productA.Price, productB.Price)
        correctAnswerForResponse = round.CorrectAnswer

    case models.RoundTypeGuess:
        guessed, parseErr := strconv.ParseFloat(req.Answer, 64)
        if parseErr != nil || guessed <= 0 {
            c.Error(ErrBadRequest("guess answer must be a positive number"))
            return
        }
        isCorrect, points = game.EvalGuess(guessed, productA.Price)
        correctAnswerForResponse = game.FormatCorrectGuessAnswer(productA.Price)

    default:
        c.Error(ErrInternal())
        return
    }

    // Transaction: insert answer, bump current_round, and if this was the
    // final round, transition to finished — all atomically.
    tx, err := h.Pool.BeginTx(ctx, pgx.TxOptions{})
    if err != nil {
        h.Logger.Error("begin tx", "err", err)
        c.Error(ErrInternal())
        return
    }
    committed := false
    defer func() {
        if !committed {
            _ = tx.Rollback(context.Background())
        }
    }()

    if _, err := h.Answers.Create(ctx, tx, round.ID, playerID, req.Answer, isCorrect, points); err != nil {
        var pgErr *pgconn.PgError
        if errors.As(err, &pgErr) && pgErr.Code == "23505" {
            // Duplicate answer for (round_id, player_id). Roll back and return
            // the existing answer with a 409.
            _ = tx.Rollback(context.Background())
            existing, getErr := h.Answers.GetByRoundID(context.Background(), round.ID)
            if getErr != nil || len(existing) == 0 {
                h.Logger.Error("duplicate answer but GetByRoundID empty", "err", getErr)
                c.Error(ErrInternal())
                return
            }
            // Pick the matching player's answer.
            for _, a := range existing {
                if a.PlayerID == playerID {
                    c.Error(ErrConflict("already_answered", "answer already submitted").
                        With("is_correct", a.IsCorrect).
                        With("points", a.PointsEarned).
                        With("correct_answer", correctAnswerForResponse))
                    return
                }
            }
            c.Error(ErrInternal())
            return
        }
        h.Logger.Error("answers.Create", "err", err)
        c.Error(ErrInternal())
        return
    }

    if err := h.Sessions.IncrementCurrentRound(ctx, tx, session.ID); err != nil {
        h.Logger.Error("increment current_round", "err", err)
        c.Error(ErrInternal())
        return
    }

    if round.RoundNumber == session.RoundsTotal {
        if err := h.Sessions.UpdateStatus(ctx, tx, session.ID, models.GameStatusFinished); err != nil {
            h.Logger.Error("transition to finished", "err", err)
            c.Error(ErrInternal())
            return
        }
    }

    if err := tx.Commit(ctx); err != nil {
        h.Logger.Error("commit answer tx", "err", err)
        c.Error(ErrInternal())
        return
    }
    committed = true

    c.JSON(http.StatusOK, AnswerResponse{
        IsCorrect:     isCorrect,
        Points:        points,
        CorrectAnswer: correctAnswerForResponse,
    })
}
```

Add the following imports at the top of `game_handlers.go` if not already present:

```go
"strconv"

"github.com/jackc/pgx/v5/pgconn"

"github.com/jzy/howmuchyousay/internal/game"
```

And make sure `"errors"` is imported (for `errors.As`) and `"github.com/jackc/pgx/v5"` for `pgx.TxOptions`.

Remove the `PostAnswer` stub from `server.go`.

- [ ] **Step 4: Run — expect PASS**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/server/ -run TestPostAnswer -v`

Expected: all PostAnswer tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/jzy/projects/howmuchyousay
git add backend/internal/server/game_handlers.go backend/internal/server/server.go backend/internal/server/post_answer_test.go
git commit -m "$(cat <<'EOF'
feat(server): POST answer with transactional advance

Inserts the answer, increments current_round, and (for the final
round) flips status to finished — all atomically in a single pgx
transaction. Validates answer format per round type, catches pgx
23505 unique-violation as already_answered 409 returning the
existing result, and only advances on commit success.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 13: GetResults handler + composition root

**Files:**
- Modify: `backend/internal/server/game_handlers.go`
- Modify: `backend/internal/server/server.go`
- Create: `backend/internal/server/get_results_test.go`
- Modify: `backend/cmd/server/main.go`

- [ ] **Step 1: Write the failing tests for GetResults**

Create `backend/internal/server/get_results_test.go`:

```go
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

    // Play through all rounds.
    for i := 1; i <= 10; i++ {
        correct := loadCorrectAnswer(t, h, sid, i)
        w := postJSON(t, h, "/api/game/"+sid.String()+"/round/"+fmt.Sprint(i)+"/answer", map[string]any{"answer": correct})
        require.Equal(t, http.StatusOK, w.Code, "round %d", i)
    }

    // Sanity: session must be finished.
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
```

- [ ] **Step 2: Run — expect FAIL**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/server/ -run TestGetResults -v`

Expected: FAIL — GetResults returns 501.

- [ ] **Step 3: Implement `GetResults`**

Append to `backend/internal/server/game_handlers.go`:

```go
// GetResults handles GET /api/game/:session_id/results.
func (h *Handler) GetResults(c *gin.Context) {
    var params SessionIDParams
    if err := c.ShouldBindUri(&params); err != nil {
        c.Error(ErrBadRequest(err.Error()))
        return
    }
    ctx := c.Request.Context()

    session, err := h.Sessions.GetByID(ctx, params.SessionID)
    if err != nil {
        h.Logger.Error("sessions.GetByID", "err", err)
        c.Error(ErrInternal())
        return
    }
    if session == nil {
        c.Error(ErrNotFound("session"))
        return
    }
    if session.Status != models.GameStatusFinished {
        c.Error(ErrConflict("game_not_finished", "game is not finished").
            With("status", string(session.Status)))
        return
    }

    players, err := h.Players.GetBySessionID(ctx, session.ID)
    if err != nil {
        h.Logger.Error("players.GetBySessionID", "err", err)
        c.Error(ErrInternal())
        return
    }
    rounds, err := h.Rounds.GetBySessionID(ctx, session.ID)
    if err != nil {
        h.Logger.Error("rounds.GetBySessionID", "err", err)
        c.Error(ErrInternal())
        return
    }

    // Aggregate answers across all rounds in the session.
    var answers []models.Answer
    for _, r := range rounds {
        roundAnswers, err := h.Answers.GetByRoundID(ctx, r.ID)
        if err != nil {
            h.Logger.Error("answers.GetByRoundID", "err", err)
            c.Error(ErrInternal())
            return
        }
        answers = append(answers, roundAnswers...)
    }

    rankings := game.CalcResults(players, answers, rounds)
    c.JSON(http.StatusOK, ResultsResponse{
        SessionID: session.ID,
        Rankings:  rankings,
    })
}
```

Remove the `GetResults` stub from `server.go`.

- [ ] **Step 4: Run GetResults tests — expect PASS**

Run: `cd backend && TEST_DATABASE_URL="postgres://hmys:hmys_test@localhost:5433/howmuchyousay_test?sslmode=disable" go test ./internal/server/ -run TestGetResults -v`

Expected: PASS.

- [ ] **Step 5: Rewrite `cmd/server/main.go` as the composition root**

Replace the contents of `backend/cmd/server/main.go`:

```go
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

    if err := store.RunMigrations(cfg.DatabaseURL, "./migrations"); err != nil {
        log.Fatalf("migrations: %v", err)
    }

    pool, err := store.ConnectDB(cfg.DatabaseURL)
    if err != nil {
        log.Fatalf("db: %v", err)
    }
    defer pool.Close()

    // Wire the real crawler (mirrors cmd/crawler/main.go).
    firecrawlClient, err := crawler.NewFirecrawlClient(cfg.FirecrawlAPIKey, cfg.FirecrawlAPIURL)
    if err != nil {
        log.Fatalf("firecrawl client: %v", err)
    }
    orch := crawler.NewOrchestrator(cfg.OpenAIAPIKey, cfg.OpenAIModel, "", firecrawlClient)
    crawlerImpl := crawler.New(
        firecrawlClient,
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
```

**Signatures to verify before pasting:** make sure `crawler.NewFirecrawlClient` and `crawler.NewOrchestrator` signatures match what's in `cmd/crawler/main.go` today. If the real `cmd/crawler/main.go` wires the crawler differently (e.g. different constructor order), mirror that file exactly — it's the authoritative reference.

- [ ] **Step 6: Build the server binary**

Run: `cd backend && go build -o /tmp/howmuchyousay-server ./cmd/server/`

Expected: success, no output.

- [ ] **Step 7: Commit**

```bash
cd /home/jzy/projects/howmuchyousay
git add backend/internal/server/game_handlers.go backend/internal/server/server.go backend/internal/server/get_results_test.go backend/cmd/server/main.go
git commit -m "$(cat <<'EOF'
feat(server): GET /results + cmd/server composition root

Implements GET /api/game/:session_id/results (returns game.CalcResults
once status=finished) and turns cmd/server/main.go from a stub into
the full composition root: migrations, pool, crawler wiring, Deps,
http.Server with graceful SIGINT/SIGTERM shutdown.

Closes out the vertical slice — the single-player game is playable
end-to-end over HTTP.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 14: Full suite verification + cleanup

**Files:** none modified unless something fails.

- [ ] **Step 1: Run the full test suite**

Run: `make test`

Expected: all packages pass. The `internal/server` suite should contain ~20 tests (CreateGame, GetSession, GetRound, PostAnswer, GetResults, error middleware).

- [ ] **Step 2: Run `go vet` across the repo**

Run: `cd backend && go vet ./...`

Expected: no output (zero warnings).

- [ ] **Step 3: Run the full build**

Run: `make build`

Expected: `bin/server` and `bin/crawler` produced, no errors.

- [ ] **Step 4: Check for leftover stubs**

Run: `cd backend && grep -n "c.Status(501)" internal/server/`

Expected: no matches. Every handler stub has been replaced.

Also run: `cd backend && grep -n "TODO\|FIXME" internal/server/`

Expected: no matches (unless explicitly annotated as an out-of-scope follow-up that matches §14 of the spec).

- [ ] **Step 5: Sanity-smoke the server (optional but recommended)**

Run: `cd backend && go run ./cmd/server/ &`

Wait ~2 seconds for startup. In a second shell:
`curl -sS -X POST http://localhost:8080/api/game -H 'Content-Type: application/json' -d '{"nick":"smoke","shop_url":"https://example.com","game_mode":"comparison","skip_crawl":true}'`

Expected: `{"error":"shop has fewer than 20 products","code":"not_enough_products","count":0}` (the dev DB is empty, so this is the correct response — it proves routing, handlers, the middleware, and the error shape all work.)

Stop the server: `kill %1` (or Ctrl-C if running in foreground).

- [ ] **Step 6: Close out**

If everything passes, there is nothing to commit — this task is verification only. If any step surfaces a regression, fix it in a follow-up commit naming the specific failure.

---

## Spec coverage map

| Spec §                         | Implemented in task(s) |
|--------------------------------|------------------------|
| §3 Architecture / package layout | 3, 4, 6, 7, 8, 13 |
| §4 Handler + Deps               | 3, 4 |
| §5 Crawler interface            | 4 (interim), 6 (final home) |
| §6 Composition root (main.go)   | 13 |
| §7 DTOs                         | 5 |
| §8.1 POST /api/game skip_crawl=true | 8 |
| §8.1 POST /api/game skip_crawl=false + runCrawlForSession | 9 |
| §8.2 GET /api/game/:id          | 10 |
| §8.3 GET /api/game/:id/round/:n + ready→in_progress | 11 |
| §8.4 POST answer + tx + duplicate handling + finish transition | 12 |
| §8.5 GET /results               | 13 |
| §9 State machine                | 8, 9, 11, 12 |
| §10 Error handling              | 4 (middleware), every handler (usage) |
| §11 Migration 008 + model + store | 1, 2 |
| §12 current_round rationale     | 1 (column), 12 (advance) |
| §13 Testing strategy (real DB + fake crawler) | 6 (harness), 8–13 (test suites) |
| §14 Out-of-scope follow-ups     | Intentionally unimplemented — flagged in spec |
