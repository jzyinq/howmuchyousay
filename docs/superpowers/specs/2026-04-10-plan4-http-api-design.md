# Plan 4 — HTTP API (Single-Player) Design

**Status:** Approved, ready for implementation planning
**Date:** 2026-04-10
**Scope:** Single-player REST API for the game. Multiplayer rooms and WebSocket are out of scope (Plan 5).
**Supersedes / depends on:** Plan 3 (game engine), Plan 1 (store), the AI-driven / Firecrawl / parallel-crawl-loop plans (crawler).

---

## 1. Goal

Turn the currently-stub `cmd/server/main.go` into a working single-player HTTP server that exposes the game end-to-end: create a session (optionally kicking off a crawl), poll session status, fetch rounds, submit answers, and read results. This is the smallest vertical slice that makes the existing `game` + `store` + `crawler` packages playable.

## 2. Non-goals

- Multiplayer rooms, lobby, or WebSocket hub — Plan 5.
- Authentication / user accounts — the spec explicitly says anonymous.
- Crawl-job durability across server restarts — acceptable follow-up if it bites us.
- Global concurrency cap on crawls — can be added later with a semaphore.
- Frontend / SPA — separate plan.

## 3. Architecture

**New package `backend/internal/server`** houses everything HTTP. `cmd/server/main.go` becomes a thin composition root that wires dependencies and starts an `http.Server` with graceful shutdown. This mirrors the existing shape of `cmd/crawler/main.go` → `internal/crawler`.

**HTTP framework:** Gin (`github.com/gin-gonic/gin`). Per the official docs (`gin-gonic.com`), we follow the struct-based handler injection pattern. Standard library `net/http` is used only for the `http.Server` wrapper around `router.Handler()` to support graceful shutdown via `srv.Shutdown(ctx)`.

**Package layout:**

```
backend/
├── cmd/server/
│   └── main.go                       # MODIFIED: wires deps, starts *http.Server, SIGINT/SIGTERM shutdown
│
├── internal/server/                  # NEW
│   ├── server.go                     # Handler struct, New(Deps), Routes() *gin.Engine
│   ├── game_handlers.go              # CreateGame, GetSession, GetRound, PostAnswer, GetResults
│   ├── crawl_runner.go               # Crawler interface + runCrawlForSession goroutine
│   ├── rounds.go                     # Helper: generate+persist rounds for a session
│   ├── errors.go                     # APIError type + error middleware
│   ├── dto.go                        # Request/response structs with binding tags
│   ├── testhelper_test.go            # setupTestHandler + fakeCrawler
│   └── *_test.go                     # Integration tests (real DB, fake crawler)
│
├── internal/models/
│   └── game.go                       # MODIFIED: add GameStatusFailed, ErrorMessage, CurrentRound
│
├── internal/store/
│   └── game_store.go                 # MODIFIED: add IncrementCurrentRound + SetErrorMessage methods
│
└── migrations/
    ├── 008_plan4_http_api.up.sql     # NEW
    └── 008_plan4_http_api.down.sql   # NEW
```

**Why one package rather than one-per-resource?** Plan 4 has 5 endpoints all operating on a single resource tree (the game session). Splitting into `internal/server/game`, `internal/server/middleware`, etc. would be premature. When Plan 5 adds rooms/WebSocket, the package can either grow (still one resource domain) or split cleanly along `game` vs `room` lines.

## 4. The `Handler` struct and dependency wiring

Struct-based handler injection, as documented by Gin for medium-to-large apps with shared dependencies:

```go
// internal/server/server.go

type Deps struct {
    Pool     *pgxpool.Pool // needed by handlers that begin their own transactions (e.g. PostAnswer)
    Sessions *store.GameStore
    Shops    *store.ShopStore
    Players  *store.PlayerStore
    Products *store.ProductStore
    Rounds   *store.RoundStore
    Answers  *store.AnswerStore
    Crawler  Crawler
    Config   *config.Config
    Logger   *slog.Logger
    Rng      *rand.Rand // math/rand/v2
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
        api.POST("/game",                                  h.CreateGame)
        api.GET("/game/:session_id",                       h.GetSession)
        api.GET("/game/:session_id/round/:number",         h.GetRound)
        api.POST("/game/:session_id/round/:number/answer", h.PostAnswer)
        api.GET("/game/:session_id/results",               h.GetResults)
    }
    return r
}
```

**Stores stay concrete.** Only one interface is introduced in this plan — the `Crawler` interface (see below) — because it's the only dependency that must be faked in tests (the stores have their own integration tests against the real DB and are reused directly). Go idiom: accept interfaces, return structs; interfaces earn their place when there's a real alternative implementation.

**Logger:** `*slog.Logger` (stdlib). No new dependency.

**RNG:** `*rand.Rand` from `math/rand/v2`, injected so tests can pass `rand.New(rand.NewPCG(seed, 0))` for deterministic round generation. The `game.GenerateComparisonRounds` / `GenerateGuessRounds` functions already take a `*rand.Rand`.

**Config:** reused from `internal/config`. Crawl timeout and Firecrawl safety cap come from there.

## 5. The `Crawler` interface

Lives in `internal/server/crawl_runner.go` — defined in the consumer package per Go idiom:

```go
// Crawler is the subset of *crawler.Crawler that the server depends on.
type Crawler interface {
    Run(ctx context.Context, cfg crawler.CrawlConfig, sessionID *uuid.UUID) (*crawler.CrawlResult, error)
}
```

The real `*crawler.Crawler.Run` already matches this signature (`backend/internal/crawler/crawler.go:93`), so no changes to the crawler package are required. Tests inject a `fakeCrawler` whose `Run` method is a configurable closure that seeds the DB with a shop/crawl/products and returns a synthetic `CrawlResult`, or returns an error to exercise the failure path.

## 6. Composition root (`cmd/server/main.go`)

Becomes ~80–100 lines of pure wiring:

```go
func main() {
    cfg := config.Load()

    if err := store.RunMigrations(cfg.DatabaseURL, "./migrations"); err != nil {
        log.Fatalf("migrations: %v", err)
    }
    pool, err := store.ConnectDB(cfg.DatabaseURL)
    if err != nil { log.Fatalf("db: %v", err) }
    defer pool.Close()

    // Construct the real crawler (same wiring as cmd/crawler/main.go).
    firecrawlClient, _ := crawler.NewFirecrawlClient(cfg.FirecrawlAPIKey, cfg.FirecrawlAPIURL)
    orch := crawler.NewOrchestrator(cfg.OpenAIAPIKey, cfg.OpenAIModel, "", firecrawlClient)
    crawlerImpl := crawler.New(
        firecrawlClient, orch,
        store.NewShopStore(pool), store.NewCrawlStore(pool), store.NewProductStore(pool),
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
        Handler: h.Routes().Handler(),
    }

    go func() {
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("listen: %v", err)
        }
    }()
    log.Printf("Server listening on %s", srv.Addr)

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

**Note on in-flight crawl goroutines during shutdown:** `srv.Shutdown` drains in-flight *HTTP requests* but not detached goroutines. Crawl goroutines use `context.Background()` (see §8) and continue until their own `CrawlTimeout` fires or they complete. For Plan 4 this is acceptable. If we later want shutdown to cancel in-flight crawls, we introduce a server-level cancellable context and thread it into `runCrawlForSession`.

## 7. Request/response DTOs

All in `internal/server/dto.go`. Gin `binding` tags provide declarative validation; `ShouldBindJSON` / `ShouldBindUri` surface failures via `c.Error(ErrBadRequest(err.Error()))`.

```go
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
    ErrorMessage *string           `json:"error_message,omitempty"` // populated when status=failed
}

type RoundResponse struct {
    Number   int              `json:"number"`
    Type     models.RoundType `json:"type"`
    ProductA ProductDTO       `json:"product_a"`
    ProductB *ProductDTO      `json:"product_b,omitempty"` // nil for guess
}

type ProductDTO struct {
    ID       uuid.UUID `json:"id"`
    Name     string    `json:"name"`
    ImageURL string    `json:"image_url"`
    // Price intentionally omitted — players must not see prices before answering.
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
```

**Why `ProductDTO` excludes `Price`:** round fetches must not leak the correct answer. Prices are only revealed in the `AnswerResponse.CorrectAnswer` (for guess) or implicitly by the `is_correct` boolean (for comparison).

## 8. Endpoint behavior

### `POST /api/game`

1. `ShouldBindJSON(&req)` → 400 on validation failure.
2. Find-or-create shop via `ShopStore.GetByURL` / `Create`.
3. **If `skip_crawl=true` (synchronous path):**
   1. `ProductStore.CountByShopID(shopID)` → if `< 20`, 409 `{code: "not_enough_products"}`.
   2. `ProductStore.GetRandomByShopID(shopID, N)` where N is sized for the game mode (comparison needs `2 * rounds_total` unique products, guess needs `rounds_total`; safe upper bound: `2 * rounds_total`).
   3. Generate round definitions in memory via `game.GenerateComparisonRounds` / `GenerateGuessRounds` with `h.Rng`. If this fails (e.g. `ErrNotEnoughValidPairs`), return 409 `{code: "round_generation_failed"}` — no DB state has been written yet.
   4. **Open a pgx transaction and perform all mutations inside it**, so a failure at any step rolls back the whole create:
      1. Insert `game_sessions` row with `status=in_progress`, `current_round=1`.
      2. Insert host `players` row.
      3. Insert `rounds` rows in order.
      4. Commit.
   5. On transaction failure → 500. Nothing is persisted, so there is no orphan cleanup concern.
   6. Return `201 Created` with `{session_id}`.
4. **If `skip_crawl=false` (async path):**
   1. Create session with `status=crawling`.
   2. Create host player row.
   3. Spawn goroutine: `go h.runCrawlForSession(sessionID, shopID, shopURL, gameMode)`.
   4. Return `201 Created` with `{session_id}` immediately.

**`runCrawlForSession` goroutine (in `crawl_runner.go`):**

```go
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
        h.markSessionFailed(context.Background(), sessionID, err.Error())
        return
    }

    // SetCrawlID, fetch products, generate + persist rounds.
    if err := h.Sessions.SetCrawlID(context.Background(), sessionID, result.CrawlID); err != nil {
        h.markSessionFailed(context.Background(), sessionID, "set crawl id: "+err.Error())
        return
    }

    products, err := h.Products.GetByShopID(context.Background(), shopID)
    if err != nil || len(products) < 20 {
        h.markSessionFailed(context.Background(), sessionID, "not enough products after crawl")
        return
    }

    if err := h.generateAndPersistRounds(context.Background(), sessionID, mode, products); err != nil {
        h.markSessionFailed(context.Background(), sessionID, "round generation: "+err.Error())
        return
    }

    if err := h.Sessions.UpdateStatus(context.Background(), sessionID, models.GameStatusReady); err != nil {
        // Rounds are already persisted, but the session is wedged in "crawling".
        // Surface it to the client via the failure path rather than leaving it stuck.
        h.markSessionFailed(context.Background(), sessionID, "mark ready: "+err.Error())
        return
    }
}
```

**Key invariant:** the goroutine uses `context.Background()`-derived contexts, NOT `c.Request.Context()`, because the HTTP request returned the moment we spawned the goroutine and its context was cancelled. Using the request context would kill the crawl immediately.

### `GET /api/game/:session_id`

1. `ShouldBindUri(&params)` → 400 on invalid UUID.
2. `GameStore.GetByID(ctx, sessionID)` → 404 if `pgx.ErrNoRows`.
3. Return `SessionResponse` populated directly from the row. `CurrentRound` is read from the column. `ErrorMessage` set when `status=failed`.

No side effects. Used for polling — this is where the client sees `crawling → ready → in_progress → finished` (or `failed`) transitions.

### `GET /api/game/:session_id/round/:number`

1. `ShouldBindUri` → 400 on invalid UUID / non-positive number.
2. `GameStore.GetByID` → 404.
3. **Status gate:** if `status ∉ {ready, in_progress}` → 409 `{code: "session_not_playable", status: "..."}`.
4. **Current-round gate:** if `number != session.CurrentRound` → 409 `{code: "not_current_round", current_round: N}`.
5. `RoundStore.GetBySessionAndNumber(sessionID, number)` → 404 if missing (shouldn't happen once rounds are persisted; defensive).
6. Load Product A (and Product B if comparison) via `ProductStore`. Map into `RoundResponse` without prices.
7. **If `status == ready`:** call `GameStore.UpdateStatus(in_progress)`. This is the implicit `ready → in_progress` transition. Done *after* the round fetch succeeds so a failed fetch leaves the session recoverable.
8. Return `200 OK`.

### `POST /api/game/:session_id/round/:number/answer`

1. `ShouldBindUri` → 400; `ShouldBindJSON(&req)` → 400.
2. `GameStore.GetByID` → 404.
3. **Status gate:** if `status != in_progress` → 409 `{code: "session_not_in_progress", status: "..."}`.
4. **Current-round gate:** if `number != session.CurrentRound` → 409 `{code: "not_current_round", current_round: N}`.
5. `RoundStore.GetBySessionAndNumber` → 404.
6. Load products.
7. **Answer format validation** (cannot be expressed in binding tags because it depends on `round.RoundType`):
   - `comparison`: `req.Answer` must be exactly `"a"` or `"b"` → 400 otherwise.
   - `guess`: `strconv.ParseFloat(req.Answer, 64)` must succeed and result `> 0` → 400 otherwise.
8. Call `game.EvalComparison` or `game.EvalGuess` to get `(isCorrect, points)`.
9. **Inside a pgx transaction** (`tx, _ := h.Pool.BeginTx(ctx, pgx.TxOptions{})`; `defer tx.Rollback(ctx)` until explicit commit):
   1. `AnswerStore.Create(ctx, tx, roundID, playerID, answer, isCorrect, points)`.
   2. `GameStore.IncrementCurrentRound(ctx, tx, sessionID)`.
   3. If this was the last round (`round.Number == session.RoundsTotal`), also `GameStore.UpdateStatus(ctx, tx, sessionID, finished)`.
   4. `tx.Commit(ctx)`.
10. **Duplicate handling:** if the `AnswerStore.Create` fails with a pgx unique-violation error (`23505`, constraint name `answers_round_player_unique`), abort the transaction, load the existing answer via `AnswerStore.GetByRoundID`, and return it as the response with **409 Conflict** `{code: "already_answered", is_correct, points, correct_answer}`.
11. Return `200 OK` with `{is_correct, points, correct_answer}`. For guess mode, `correct_answer` uses `game.FormatCorrectGuessAnswer(round.ProductA.Price)` for consistent client display.

**Why a transaction for step 9:** ensures the `answers` row, the `current_round` increment, and (for the final round) the `finished` status update all land atomically. Without it, a crash between the insert and the increment could leave the session wedged, and a crash between the increment and the finish-status update could leave the last round played but not marked finished.

**Plan-5 handoff note:** the inline `IncrementCurrentRound` + `UpdateStatus(finished)` logic in this handler is the single-player placeholder for the multiplayer barrier. Plan 5 will replace this with a barrier service that (a) increments only when all players have answered round N or the 30s timeout fires, (b) emits the `answer_count` and `round_result` WebSocket events around the barrier. The `current_round` column itself and its semantics ("the round everyone is currently playing") survive unchanged.

### `GET /api/game/:session_id/results`

1. `ShouldBindUri` → 400.
2. `GameStore.GetByID` → 404.
3. If `status != finished` → 409 `{code: "game_not_finished", status: "..."}`.
4. Load players, rounds, answers for the session.
5. Call `game.CalcResults(players, answers, rounds)` → `[]PlayerScore`.
6. Return `200 OK` with `{session_id, rankings}`.

Pure read, no state changes.

## 9. State machine

All single-player transitions are **monotonic and forward-only**. No transition out of `finished` or `failed`. The `lobby` state is untouched in Plan 4.

| From         | Event                                           | To            | Side effect                |
|--------------|--------------------------------------------------|---------------|----------------------------|
| (none)       | `POST /api/game` `skip_crawl=true`              | `in_progress` | `current_round=1`          |
| (none)       | `POST /api/game` `skip_crawl=false`             | `crawling`    | `current_round=1`          |
| `crawling`   | crawl goroutine success + rounds persisted      | `ready`       | —                          |
| `crawling`   | crawl error / insufficient products / round-gen error | `failed`  | `error_message` set        |
| `ready`      | first successful `GET /round/1`                 | `in_progress` | —                          |
| `in_progress`| any answer persisted                            | `in_progress` | `current_round++`          |
| `in_progress`| answer for round `rounds_total` persisted       | `finished`    | (same transaction)         |

`current_round` is a stored column on `game_sessions`, incremented atomically with answer insertion. See §11 for the rationale (multiplayer compatibility).

## 10. Error handling

**Typed API error (`internal/server/errors.go`):**

```go
type APIError struct {
    Status  int
    Code    string         // machine-readable, e.g. "not_current_round"
    Message string         // human-readable
    Details map[string]any // optional context
}

func (e *APIError) Error() string { return e.Message }
func (e *APIError) With(k string, v any) *APIError {
    if e.Details == nil { e.Details = map[string]any{} }
    e.Details[k] = v
    return e
}

func ErrBadRequest(msg string) *APIError        { return &APIError{Status: 400, Code: "bad_request", Message: msg} }
func ErrNotFound(what string) *APIError         { return &APIError{Status: 404, Code: "not_found", Message: what + " not found"} }
func ErrConflict(code, msg string) *APIError    { return &APIError{Status: 409, Code: code, Message: msg} }
func ErrInternal(err error) *APIError           { return &APIError{Status: 500, Code: "internal", Message: "internal server error"} }
```

**Handler usage:** handlers call `c.Error(apiErr)` and `return`. They never call `c.JSON(...)` for error cases.

```go
if errors.Is(err, pgx.ErrNoRows) {
    c.Error(ErrNotFound("session"))
    return
}
if params.Number != session.CurrentRound {
    c.Error(ErrConflict("not_current_round", "not current round").
        With("current_round", session.CurrentRound))
    return
}
```

**Middleware** (`errorMiddleware()`):

```go
func (h *Handler) errorMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Next()
        if len(c.Errors) == 0 { return }
        last := c.Errors.Last().Err

        var apiErr *APIError
        if errors.As(last, &apiErr) {
            body := gin.H{"error": apiErr.Message, "code": apiErr.Code}
            for k, v := range apiErr.Details { body[k] = v }
            c.AbortWithStatusJSON(apiErr.Status, body)
            return
        }
        h.Logger.Error("unhandled handler error", "err", last)
        c.AbortWithStatusJSON(500, gin.H{"error": "internal server error", "code": "internal"})
    }
}
```

**Uniform error shape:**
```json
{ "error": "not current round", "code": "not_current_round", "current_round": 3 }
```

The `code` is machine-readable — clients branch on it without regex-matching `error`. Extra context fields merged at top level.

**HTTP status conventions:**
- `400` — request validation failure (bad JSON, missing field, wrong type, invalid UUID, unsupported game mode, invalid answer format).
- `404` — session / round / player not found.
- `409` — valid request but wrong state (session not in progress, not current round, already answered, game not finished, not enough products for skip_crawl).
- `500` — unexpected server-side failure (DB errors, logic bugs).

## 11. Data model changes

### Migration `008_plan4_http_api`

**Up (`008_plan4_http_api.up.sql`):**
```sql
-- Add "failed" to the game_status enum.
-- PostgreSQL 12+ permits ALTER TYPE ADD VALUE inside a transaction as long as the new
-- value is not referenced in the same transaction. This migration only adds the value;
-- it does not INSERT or UPDATE any row to 'failed', so running under golang-migrate's
-- default transactional wrapping is safe. Both the dev and test databases run
-- postgres:16-alpine (docker-compose.yml), well above the PG 12 minimum.
ALTER TYPE game_status ADD VALUE IF NOT EXISTS 'failed';

-- Error message column populated when a session transitions to "failed".
ALTER TABLE game_sessions ADD COLUMN error_message TEXT;

-- Current round tracking. Starts at 1; advances per §9.
-- In single-player (Plan 4), advanced inline in the answer handler.
-- In multiplayer (Plan 5), advanced by the barrier service.
ALTER TABLE game_sessions ADD COLUMN current_round INT NOT NULL DEFAULT 1;

-- Prevent double-answers at the DB layer. Complements the application-level
-- current-round check (which is the primary defense) as a race-condition safety net.
ALTER TABLE answers ADD CONSTRAINT answers_round_player_unique UNIQUE (round_id, player_id);
```

**Down (`008_plan4_http_api.down.sql`):**
```sql
ALTER TABLE answers DROP CONSTRAINT IF EXISTS answers_round_player_unique;
ALTER TABLE game_sessions DROP COLUMN IF EXISTS current_round;
ALTER TABLE game_sessions DROP COLUMN IF EXISTS error_message;
-- Note: PostgreSQL does not support removing a value from an ENUM.
-- Full downgrade requires recreating the game_status type; this down migration is best-effort for dev rollbacks.
```

### Model changes (`internal/models/game.go`)

```go
const (
    // ...existing statuses...
    GameStatusFailed GameStatus = "failed"
)

type GameSession struct {
    // ...existing fields...
    CurrentRound int     `json:"current_round"`
    ErrorMessage *string `json:"error_message,omitempty"`
}
```

### Store additions (`internal/store/game_store.go`)

```go
// IncrementCurrentRound atomically increments current_round by 1.
// Can be called inside or outside a pgx transaction (takes a context, uses the store's pool).
func (s *GameStore) IncrementCurrentRound(ctx context.Context, id uuid.UUID) error {
    _, err := s.pool.Exec(ctx,
        `UPDATE game_sessions SET current_round = current_round + 1, updated_at = NOW() WHERE id = $1`,
        id)
    return err
}

// SetErrorMessage writes the error_message field (used when transitioning to "failed").
func (s *GameStore) SetErrorMessage(ctx context.Context, id uuid.UUID, msg string) error {
    _, err := s.pool.Exec(ctx,
        `UPDATE game_sessions SET error_message = $2, updated_at = NOW() WHERE id = $1`,
        id, msg)
    return err
}
```

`GetByID` and the `scanSession` helper must be updated to read the new columns.

**Transaction support for the answer handler.** The answer handler must atomically (a) insert the `answers` row, (b) increment `current_round`, and (c) if this was the last round, update `status` to `finished`. To support this, the spec requires:

1. A shared `store.DBExec` interface covering the pgx methods used by these three stores (`Exec`, `QueryRow`, `Query`). Both `*pgxpool.Pool` and `pgx.Tx` satisfy this — pgx provides it via the `DBTX` pattern.
2. `AnswerStore.Create`, `GameStore.IncrementCurrentRound`, and `GameStore.UpdateStatus` accept a `DBExec` parameter (or gain `*Tx`-named variants). The existing call sites in other handlers and tests pass the store's pool.
3. `Deps` includes `Pool *pgxpool.Pool` so the answer handler can begin the transaction via `pool.BeginTx(ctx, pgx.TxOptions{})` and pass the resulting `pgx.Tx` to the three store calls.

The implementation plan picks the exact signature style (parameter vs. suffix-named methods) but the atomicity requirement and the `DBExec` interface are fixed by this spec.

## 12. Why `current_round` is stored, not derived

A derived definition (`count(answers) + 1`) happens to work for single-player because there is exactly one player. In multiplayer, the spec's round barrier (§Bariera synchronizacji rund, lines 117–131 of the main design spec) makes `current_round` a property of the *session*: everyone plays the same round in lockstep, and the round only advances when all players have answered or the 30s timeout fires. A fast player's answer count is not the same as the session's current round.

Storing `current_round` on `game_sessions` now means Plan 5 doesn't have to rip out and replace a derived computation — only the transition *trigger* changes (from "inline after any answer" to "when barrier closes"). The column semantics are identical in both plans.

## 13. Testing strategy

**Level:** full integration tests against the real PostgreSQL test database, with a fake crawler. Same infrastructure as `internal/store`: `postgres-test` docker compose service, `TEST_DATABASE_URL`, `store.RunMigrations` on setup. The new test package hits the same shared test DB; the `-p 1` serial-execution flag in the Makefile (added today) prevents cross-package races.

**Test helper (`internal/server/testhelper_test.go`):**

```go
func setupTestHandler(t *testing.T) (*Handler, *pgxpool.Pool, *fakeCrawler) {
    t.Helper()
    pool := setupTestDB(t) // migrations + cleanup registered via t.Cleanup
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
```

`setupTestDB` and `cleanupTestDB` follow the same shape as `store/testhelper_test.go` — reuse the DB URL logic, run migrations, register cleanup. The `cleanupTestDB` table list is extended as needed.

**Fake crawler (`internal/server/crawl_runner_test.go` or `testhelper_test.go`):**

```go
type fakeCrawler struct {
    behavior func(ctx context.Context, cfg crawler.CrawlConfig, sessionID *uuid.UUID) (*crawler.CrawlResult, error)
    done     chan struct{} // closed after behavior returns, for test synchronization
    called   int
}

func (f *fakeCrawler) Run(ctx context.Context, cfg crawler.CrawlConfig, sid *uuid.UUID) (*crawler.CrawlResult, error) {
    f.called++
    result, err := f.behavior(ctx, cfg, sid)
    if f.done != nil { close(f.done) }
    return result, err
}
```

**Goroutine synchronization: no sleeps.** Tests that exercise the async crawl path use the fake's `done` channel:

```go
h, pool, fake := setupTestHandler(t)
fake.done = make(chan struct{})
fake.behavior = func(ctx context.Context, cfg crawler.CrawlConfig, sid *uuid.UUID) (*crawler.CrawlResult, error) {
    // Seed DB synchronously inside the fake: shop, crawl row, 20 products.
    // Return a synthetic CrawlResult.
    ...
}

w := httptest.NewRecorder()
body := bytes.NewBufferString(`{"nick":"t","shop_url":"https://x.com","game_mode":"comparison"}`)
req := httptest.NewRequest("POST", "/api/game", body)
req.Header.Set("Content-Type", "application/json")
h.Routes().ServeHTTP(w, req)
require.Equal(t, 201, w.Code)

select {
case <-fake.done:
case <-time.After(5 * time.Second):
    t.Fatal("crawl goroutine did not complete")
}

// Assert final state: session status=ready, 10 rounds persisted.
```

**Minimum test case set:**

| Test | Verifies |
|------|----------|
| `TestCreateGame_Validation` | Bad JSON, missing fields, unknown game_mode → 400 |
| `TestCreateGame_SkipCrawl_Success` | Shop + 20 products seeded → session created `in_progress`, 10 rounds persisted, player created |
| `TestCreateGame_SkipCrawl_NotEnoughProducts` | <20 products → 409 `not_enough_products` |
| `TestCreateGame_WithCrawl_Success` | Goroutine runs fake, session `crawling → ready`, 10 rounds persisted |
| `TestCreateGame_WithCrawl_Failure` | Fake returns error → session `crawling → failed`, `error_message` populated |
| `TestGetSession_Polling` | Observed sequence: `crawling` → `ready` → `in_progress` → `finished` |
| `TestGetRound_NotCurrent` | Fetching round 3 when `current_round=1` → 409 with `current_round` in response |
| `TestGetRound_ReadyToInProgress` | First successful `GET /round/1` flips `ready → in_progress` |
| `TestGetRound_FinishedSession` | GET round on finished session → 409 |
| `TestGetRound_PriceHidden` | `RoundResponse` JSON does not contain `price` anywhere |
| `TestPostAnswer_Comparison_Correct` | Correct answer → `is_correct: true`, points, `current_round` advances atomically |
| `TestPostAnswer_Comparison_InvalidFormat` | `"x"` for comparison → 400 |
| `TestPostAnswer_Guess_Correct` | Numeric parse, eval, points |
| `TestPostAnswer_Guess_InvalidFormat` | `"abc"` for guess → 400 |
| `TestPostAnswer_WrongRound` | Answering round 3 when `current_round=1` → 409 `not_current_round` |
| `TestPostAnswer_Duplicate` | Same round answered twice → 409 `already_answered` with first answer's data |
| `TestPostAnswer_FinalRound_TransitionsToFinished` | Last round's answer atomically flips status to `finished` |
| `TestPostAnswer_FinishedSession` | Answering on finished session → 409 `session_not_in_progress` |
| `TestGetResults_NotFinished` | 409 `game_not_finished` |
| `TestGetResults_Success` | Rankings returned via `game.CalcResults` |

~20 tests. Each ≈100ms against the test DB → suite runs in ~2s. Runs inside the existing `-p 1` serial flow.

## 14. Out-of-scope follow-ups (not blocking Plan 4)

These are known gaps acknowledged but deferred:

- **Restart durability:** if the server crashes mid-crawl, the session stays in `crawling` forever. Fix: add a startup sweeper that marks any `crawling` session older than `CrawlTimeout + grace` as `failed`.
- **Global crawl concurrency cap:** no semaphore limits concurrent `runCrawlForSession` goroutines. Fix when needed: `chan struct{}` buffered to N, acquired before `Crawler.Run`.
- **Crawl cancellation on shutdown:** `srv.Shutdown` does not cancel detached crawl goroutines. Fix: server-level cancellable context threaded into `runCrawlForSession`.
- **Request-level observability:** no request IDs, no structured access logs beyond `gin.Logger()`'s default format. Add when operational needs arise.
- **Rate limiting / abuse protection:** none. Not needed for a dev/side-project; revisit if the server is ever exposed publicly.

None of these blocks making the game playable end-to-end.
