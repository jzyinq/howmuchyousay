# Plan 5 — Single-Player Frontend (MVP) Design

**Status:** Approved, ready for implementation planning
**Date:** 2026-04-11
**Scope:** MVP single-player frontend SPA — comparison mode only, skip_crawl only. Multiplayer, guess mode, and crawl-polling UI are out of scope.
**Depends on:** Plan 4 (HTTP API for single-player)

---

## 1. Goal

Build a playable React SPA that lets a player create a single-player comparison game (from pre-crawled products), play through 10 rounds of "which product is more expensive?", and see their final score. This is the smallest frontend slice that makes the game playable end-to-end against the Plan 4 API.

## 2. Non-goals

- Guess mode UI — MVP is comparison only.
- Crawl-polling UI / crawl progress screen — MVP uses `skip_crawl=true` exclusively.
- Multiplayer rooms, lobby, WebSocket — separate plan.
- Authentication / user accounts — spec says anonymous.
- Mobile-first responsive design — desktop-first is fine, basic mobile usability is a bonus.
- Dark mode — neobrutalism theme supports it, but not wiring it up for MVP.

## 3. Tech Stack

| Tool | Version | Purpose |
|------|---------|---------|
| Vite | latest | Build tool, dev server |
| React | 18 | UI framework |
| TypeScript | 5.x | Type safety |
| React Router | v7 | Client-side SPA routing |
| TanStack Query | v5 | Server state, API calls, caching |
| Tailwind CSS | v4 | Utility-first styling |
| Neobrutalism components | latest | UI component library (shadcn/ui-based) |
| class-variance-authority | latest | Component variant management (shadcn dependency) |
| clsx + tailwind-merge | latest | Class name utilities (shadcn dependency) |

**No additional state management library.** React Router handles URL state, TanStack Query handles server state, `useState` covers local UI state.

## 4. Project Structure

```
frontend/
├── public/
├── src/
│   ├── api/
│   │   ├── client.ts            # fetch wrapper with base URL, JSON headers, error handling
│   │   └── gameApi.ts           # TanStack Query hooks: useCreateGame, useSession, useRound, useSubmitAnswer, useResults
│   ├── components/
│   │   ├── ui/                  # Neobrutalism components (copied via shadcn CLI)
│   │   │   ├── button.tsx
│   │   │   ├── input.tsx
│   │   │   ├── card.tsx
│   │   │   ├── label.tsx
│   │   │   ├── select.tsx
│   │   │   ├── progress.tsx
│   │   │   └── badge.tsx
│   │   ├── setup/
│   │   │   └── GameSetupForm.tsx  # Nick, shop URL, game mode, submit
│   │   ├── game/
│   │   │   ├── ProductCard.tsx    # Product image + name, selectable highlight
│   │   │   ├── ComparisonRound.tsx # Two ProductCards, "Which is more expensive?", pick A/B
│   │   │   ├── RoundResult.tsx    # Correct/incorrect feedback, points, prices revealed
│   │   │   ├── RoundCounter.tsx   # "Round 3/10" + progress bar
│   │   │   └── ScoreTracker.tsx   # Running total score
│   │   └── results/
│   │       └── GameSummary.tsx    # Final score, per-round breakdown, play again
│   ├── lib/
│   │   └── utils.ts             # cn() helper (clsx + tailwind-merge)
│   ├── pages/
│   │   ├── HomePage.tsx
│   │   ├── SetupPage.tsx
│   │   ├── GamePage.tsx
│   │   └── ResultsPage.tsx
│   ├── types/
│   │   └── index.ts             # Shared TypeScript types matching API DTOs
│   ├── App.tsx                  # Router setup, QueryClientProvider
│   └── main.tsx                 # Entry point
├── components.json              # shadcn/neobrutalism config
├── index.html
├── package.json
├── tsconfig.json
├── tailwind.config.ts
└── vite.config.ts
```

## 5. Routes

| Path | Page | Purpose |
|------|------|---------|
| `/` | HomePage | Landing — game title, "Play" button |
| `/play` | SetupPage | Game setup form |
| `/game/:sessionId` | GamePage | Gameplay — rounds, answers |
| `/game/:sessionId/results` | ResultsPage | Final score, breakdown |

**Navigation flow:**
1. `/` → click "Play" → `/play`
2. `/play` → submit form → `POST /api/game` (skip_crawl=true) → redirect to `/game/:sessionId`
3. `/game/:sessionId` → play rounds → after round 10 → redirect to `/game/:sessionId/results`
4. `/game/:sessionId/results` → "Play Again" → `/play`

No back-navigation during gameplay. Rounds are sequential, answers are final. Browser back from GamePage goes to `/play`.

## 6. Components

### 6.1 Neobrutalism UI Components (from shadcn CLI)

Copied into `components/ui/` via the shadcn CLI with neobrutalism variants:
- **Button** — default (colored + shadow), neutral (secondary bg + shadow), variants for primary actions and secondary actions
- **Input** — border-2, rounded-base, neobrutalism focus ring
- **Card** — border-2, shadow-shadow, rounded-base container
- **Label** — font-heading weight
- **Select** — for game mode selection (comparison pre-selected, guess disabled for MVP)
- **Progress** — for round progress bar
- **Badge** — for point display in results

All components use the neobrutalism design tokens: `rounded-base`, `border-2 border-border`, `shadow-shadow`, `bg-main`, `text-main-foreground`. Interactive elements use `hover:translate-x-boxShadowX hover:translate-y-boxShadowY hover:shadow-none` for the characteristic shadow-shift effect.

### 6.2 GameSetupForm

Form fields:
- **Nick** — text input, required, 1-32 chars
- **Shop URL** — text input, required, valid URL
- **Game Mode** — select dropdown, "Comparison" pre-selected, "Guess" shown but disabled with "(coming soon)"

Submit triggers `useCreateGame` mutation with `{nick, shop_url, game_mode: "comparison", skip_crawl: true}`. On success, navigates to `/game/:sessionId`.

Error states:
- 409 `not_enough_products` → inline message: "Not enough products in this shop's database. Try a different shop URL."
- Network error → inline message with retry

### 6.3 ProductCard

Displays a single product:
- Product image (with fallback placeholder if image fails to load)
- Product name
- Neobrutalism card styling: `border-2 border-border shadow-shadow rounded-base`
- **Selectable state:** when clicked, border color changes to `main`, gains a highlight effect
- **Result state:** after answer submitted, shows the actual price below the name

### 6.4 ComparisonRound

Layout: two ProductCards side by side (stacked on narrow screens) with a prompt "Which product is more expensive?" above them.

- Cards are clickable — clicking selects that product as the answer
- A "Submit" button below becomes active when a card is selected
- After submission, both cards transition to result state (prices revealed)

### 6.5 RoundResult

Shown after answer submission:
- Correct: green badge "Correct! +N points"
- Incorrect: red badge "Wrong! The answer was [product name]"
- Both product prices revealed on the ProductCards
- "Next Round" button (or auto-advance after 2-3 seconds — implementation detail)
- On the final round: "See Results" button instead

### 6.6 RoundCounter

Simple display: "Round 3 / 10" with a Progress bar underneath showing completion percentage.

### 6.7 ScoreTracker

Running total: "Score: 12" displayed in a Badge or small card. Updates after each round result.

### 6.8 GameSummary

Results page content:
- Total score prominently displayed
- Per-round breakdown table/list:
  - Round number
  - Products shown
  - Your pick vs correct answer
  - Points earned
- "Play Again" button → navigates to `/play`

## 7. API Integration

### 7.1 API Client (`api/client.ts`)

Thin wrapper around `fetch`:
- Base URL from Vite env var `VITE_API_URL` (defaults to empty string, proxied by Vite dev server)
- Automatic `Content-Type: application/json`
- Response parsing: checks `res.ok`, parses JSON, throws typed error on failure
- Error shape matches backend: `{error: string, code: string, ...details}`

### 7.2 TanStack Query Hooks (`api/gameApi.ts`)

```typescript
// Mutations
useCreateGame()      → POST /api/game
useSubmitAnswer(sessionId, roundNumber) → POST /api/game/:sessionId/round/:number/answer

// Queries
useSession(sessionId)               → GET /api/game/:sessionId
useRound(sessionId, roundNumber)    → GET /api/game/:sessionId/round/:number
useResults(sessionId)               → GET /api/game/:sessionId/results
```

**Query configuration:**
- `useSession`: `staleTime: Infinity` for MVP (session data doesn't change during skip_crawl flow after creation). When crawl-polling is added later, this becomes a polling query.
- `useRound`: `staleTime: Infinity`, keyed by `[sessionId, roundNumber]`. Each round is fetched once.
- `useResults`: `staleTime: Infinity`, fetched once on results page.

### 7.3 Error Handling

API errors are parsed into a typed `ApiError`:
```typescript
interface ApiError {
  status: number
  code: string
  message: string
  details?: Record<string, unknown>
}
```

Components handle errors inline:
- **409 codes** (not_enough_products, not_current_round, already_answered, session_not_in_progress) → contextual messages
- **404** → redirect to home
- **Network errors** → "Something went wrong" with retry
- **400** → form validation feedback (shouldn't happen if client validates first)

## 8. Game State Management

**No global state store.** State lives in three layers:

### 8.1 URL State (React Router)
- `sessionId` from route params identifies the game

### 8.2 Server State (TanStack Query Cache)
- Session data (status, current_round, rounds_total)
- Round data (products, round type)
- Results (rankings)

### 8.3 Local Component State (GamePage)

```typescript
selectedAnswer: "a" | "b" | null     // which card the player clicked
roundPhase: "answering" | "result"   // picking vs. viewing result
localRound: number                   // client-side round counter
totalScore: number                   // accumulated score
roundHistory: RoundHistoryEntry[]    // stored for results breakdown display
```

**Round advancement:** after submitting an answer, `localRound` increments client-side (always +1, deterministic). No need to refetch session to discover the new round number.

**Page refresh resilience:** `useSession` on mount returns `current_round` from the server, re-syncing local state. TanStack Query cache is lost on refresh, so round data re-fetches. Already-answered rounds are protected by the backend's 409 `already_answered` response.

**Round history:** each round's result (products, answer, correct answer, points) is accumulated in local state and passed to the results page via React Router's `navigate` state (`navigate(\`/game/\${sessionId}/results\`, { state: { roundHistory, totalScore } })`). The ResultsPage reads this from `useLocation().state`. If state is missing (direct URL access or refresh), it falls back to fetching `useResults` for the server-side rankings (which lack per-round detail but still show the final score). This avoids needing a separate "get all rounds with answers" API endpoint.

## 9. Vite Configuration

```typescript
// vite.config.ts
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
```

The Go backend serves on `:8080`. Vite dev server proxies `/api/*` requests, eliminating CORS during development. For production, the Go server can serve the built SPA static files or a reverse proxy (nginx) handles both.

## 10. Styling & Theme

**Neobrutalism globals.css** adapted from the library's theme (Tailwind CSS v4 syntax):

Key design tokens:
- `--border-radius: 5px` → `rounded-base`
- `--box-shadow-x: 4px` / `--box-shadow-y: 4px` → `shadow-shadow`
- `--main`: primary accent color (oklch blue by default)
- `--background`: page background
- `--secondary-background`: card/surface background
- `--border`: black borders
- `border-2 border-border` on all cards and interactive elements
- `hover:translate-x-boxShadowX hover:translate-y-boxShadowY hover:shadow-none` for button/card press effect

**Layout:** centered single-column, max-width container. No sidebar, no persistent navbar. Game title/logo at the top of each page. Clean, focused on the gameplay.

**Typography:** `font-heading` (700 weight) for headings, `font-base` (500 weight) for body text.

## 11. TypeScript Types (`types/index.ts`)

Matches the backend API DTOs from Plan 4:

```typescript
type GameMode = "comparison" | "guess"
type GameStatus = "crawling" | "ready" | "in_progress" | "finished" | "failed"

interface CreateGameRequest {
  nick: string
  shop_url: string
  game_mode: GameMode
  skip_crawl: boolean
}

interface CreateGameResponse {
  session_id: string
}

interface SessionResponse {
  id: string
  status: GameStatus
  game_mode: GameMode
  rounds_total: number
  current_round: number
  error_message?: string
}

interface ProductDTO {
  id: string
  name: string
  image_url: string
}

interface RoundResponse {
  number: number
  type: string
  product_a: ProductDTO
  product_b?: ProductDTO
}

interface AnswerResponse {
  is_correct: boolean
  points: number
  correct_answer: string
}

interface PlayerScore {
  player_id: string
  nick: string
  total_points: number
  correct_answers: number
  total_answers: number
}

interface ResultsResponse {
  session_id: string
  rankings: PlayerScore[]
}

// Client-side only
interface RoundHistoryEntry {
  round_number: number
  product_a: ProductDTO
  product_b: ProductDTO
  selected_answer: "a" | "b"
  is_correct: boolean
  points: number
  correct_answer: string
}
```

## 12. Out-of-scope follow-ups (not blocking MVP)

- **Guess mode UI** — add GuessRound component with numeric input, different scoring display.
- **Crawl-polling UI** — add a waiting/progress screen, change `useSession` to poll with `refetchInterval` while `status === "crawling"`.
- **Dark mode toggle** — neobrutalism theme supports `.dark` class, just needs a toggle wired up.
- **Multiplayer** — rooms, lobby, WebSocket, player list — separate plan entirely.
- **Mobile responsive polish** — basic stacking works, but fine-tuning card sizes, touch targets, etc.
- **Image lazy loading / error states** — placeholder images, loading skeletons.
- **Analytics / error tracking** — Sentry, PostHog, etc.
- **Production build / deployment** — static hosting, Docker, etc.
