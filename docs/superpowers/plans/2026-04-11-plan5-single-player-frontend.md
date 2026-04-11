# Single-Player Frontend (MVP) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a playable React SPA that lets a player create a single-player comparison game (from pre-crawled products), play 10 rounds of "which is more expensive?", and see their final score.

**Architecture:** Vite + React 18 + TypeScript SPA in `frontend/`. React Router v7 for 4 routes (Home, Setup, Game, Results). TanStack Query v5 for API calls. Neobrutalism.dev components (shadcn/ui + Tailwind CSS v4) for all UI. No global state — URL params + query cache + local useState.

**Tech Stack:** Vite, React 18, TypeScript 5, React Router v7, TanStack Query v5, Tailwind CSS v4, neobrutalism.dev, class-variance-authority, clsx, tailwind-merge

---

### Task 1: Scaffold Vite + React + TypeScript project

**Files:**
- Create: `frontend/package.json`
- Create: `frontend/index.html`
- Create: `frontend/src/main.tsx`
- Create: `frontend/src/App.tsx`
- Create: `frontend/tsconfig.json`
- Create: `frontend/vite.config.ts`

- [ ] **Step 1: Create Vite project**

Run from the project root:

```bash
cd /home/jzy/projects/howmuchyousay
npm create vite@latest frontend -- --template react-ts
```

- [ ] **Step 2: Install dependencies**

```bash
cd /home/jzy/projects/howmuchyousay/frontend
npm install react-router@7 @tanstack/react-query
npm install -D tailwindcss @tailwindcss/vite
```

- [ ] **Step 3: Configure Vite with Tailwind and API proxy**

Replace `frontend/vite.config.ts`:

```typescript
import { defineConfig } from "vite"
import react from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"
import path from "path"

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
})
```

- [ ] **Step 4: Set up CSS entry point**

Replace `frontend/src/index.css` with the neobrutalism globals. This is adapted from the neobrutalism.dev source (`globals.css`):

```css
@import "tailwindcss";
@import "tw-animate-css";

@custom-variant dark (&:is(.dark *));

:root {
  --border-radius: 5px;
  --box-shadow-x: 4px;
  --box-shadow-y: 4px;
  --reverse-box-shadow-x: -4px;
  --reverse-box-shadow-y: -4px;

  --heading-font-weight: 700;
  --base-font-weight: 500;

  --background: oklch(93.46% 0.0304 254.32);
  --secondary-background: oklch(100% 0 0);
  --foreground: oklch(0% 0 0);
  --main-foreground: oklch(0% 0 0);

  --main: oklch(67.47% 0.1725 259.61);
  --border: oklch(0% 0 0);
  --ring: oklch(0% 0 0);
  --overlay: oklch(0% 0 0 / 0.8);

  --shadow: var(--box-shadow-x) var(--box-shadow-y) 0px 0px var(--border);
}

@theme inline {
  --color-main: var(--main);
  --color-background: var(--background);
  --color-secondary-background: var(--secondary-background);
  --color-foreground: var(--foreground);
  --color-main-foreground: var(--main-foreground);
  --color-border: var(--border);
  --color-overlay: var(--overlay);
  --color-ring: var(--ring);

  --spacing-boxShadowX: var(--box-shadow-x);
  --spacing-boxShadowY: var(--box-shadow-y);
  --spacing-reverseBoxShadowX: var(--reverse-box-shadow-x);
  --spacing-reverseBoxShadowY: var(--reverse-box-shadow-y);

  --radius-base: var(--border-radius);

  --shadow-shadow: var(--shadow);

  --font-weight-base: var(--base-font-weight);
  --font-weight-heading: var(--heading-font-weight);
}

@layer base {
  body {
    @apply bg-background text-foreground font-base;
  }

  h1,
  h2,
  h3,
  h4,
  h5,
  h6 {
    @apply font-heading;
  }
}
```

- [ ] **Step 5: Install tw-animate-css**

```bash
cd /home/jzy/projects/howmuchyousay/frontend
npm install tw-animate-css
```

- [ ] **Step 6: Set up tsconfig path alias**

Update `frontend/tsconfig.json` (or `tsconfig.app.json` depending on Vite's scaffold) to add:

```json
{
  "compilerOptions": {
    "baseUrl": ".",
    "paths": {
      "@/*": ["./src/*"]
    }
  }
}
```

Ensure `tsconfig.json` includes the path alias. If Vite scaffolded a `tsconfig.app.json`, add the paths there too.

- [ ] **Step 7: Create utility file for cn() helper**

Create `frontend/src/lib/utils.ts`:

```typescript
import { type ClassValue, clsx } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}
```

Install the dependencies:

```bash
cd /home/jzy/projects/howmuchyousay/frontend
npm install clsx tailwind-merge class-variance-authority
```

- [ ] **Step 8: Create minimal App.tsx to verify setup**

Replace `frontend/src/App.tsx`:

```tsx
export default function App() {
  return (
    <div className="min-h-screen bg-background flex items-center justify-center">
      <h1 className="text-4xl font-heading text-foreground">
        HowMuchYouSay
      </h1>
    </div>
  )
}
```

- [ ] **Step 9: Verify it runs**

```bash
cd /home/jzy/projects/howmuchyousay/frontend
npm run dev
```

Open `http://localhost:5173` in a browser. Verify:
- Page loads with the light purple/blue neobrutalism background
- "HowMuchYouSay" displays in bold heading font
- No console errors

- [ ] **Step 10: Add frontend to .gitignore and commit**

Add `frontend/node_modules/` to the project's `.gitignore` if not already present.

```bash
cd /home/jzy/projects/howmuchyousay
git add frontend/
git commit -m "feat(frontend): scaffold Vite + React + TypeScript + Tailwind v4 + neobrutalism theme"
```

---

### Task 2: Add neobrutalism UI components (Button, Input, Card, Label, Select, Progress, Badge)

**Files:**
- Create: `frontend/src/components/ui/button.tsx`
- Create: `frontend/src/components/ui/input.tsx`
- Create: `frontend/src/components/ui/card.tsx`
- Create: `frontend/src/components/ui/label.tsx`
- Create: `frontend/src/components/ui/select.tsx`
- Create: `frontend/src/components/ui/progress.tsx`
- Create: `frontend/src/components/ui/badge.tsx`
- Create: `frontend/components.json`

These are all copy-paste from the neobrutalism.dev repository. The shadcn CLI can be used if it works, but manual copy is fine too since neobrutalism components are self-contained.

- [ ] **Step 1: Initialize shadcn**

```bash
cd /home/jzy/projects/howmuchyousay/frontend
npx shadcn@latest init
```

When prompted:
- Style: Default
- Base color: Neutral (doesn't matter for neobrutalism)
- CSS variables: Yes

This creates `components.json`. The `components/ui/` directory will be populated in the next steps.

- [ ] **Step 2: Add Button component**

Create `frontend/src/components/ui/button.tsx` with the neobrutalism variant:

```tsx
import { Slot } from "@radix-ui/react-slot"
import { cva, type VariantProps } from "class-variance-authority"
import * as React from "react"
import { cn } from "@/lib/utils"

const buttonVariants = cva(
  "inline-flex items-center justify-center whitespace-nowrap rounded-base text-sm font-base ring-offset-white transition-all gap-2 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-black focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50",
  {
    variants: {
      variant: {
        default:
          "text-main-foreground bg-main border-2 border-border shadow-shadow hover:translate-x-boxShadowX hover:translate-y-boxShadowY hover:shadow-none",
        noShadow: "text-main-foreground bg-main border-2 border-border",
        neutral:
          "bg-secondary-background text-foreground border-2 border-border shadow-shadow hover:translate-x-boxShadowX hover:translate-y-boxShadowY hover:shadow-none",
        reverse:
          "text-main-foreground bg-main border-2 border-border hover:translate-x-reverseBoxShadowX hover:translate-y-reverseBoxShadowY hover:shadow-shadow",
      },
      size: {
        default: "h-10 px-4 py-2",
        sm: "h-9 px-3",
        lg: "h-11 px-8",
        icon: "size-10",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  },
)

function Button({
  className,
  variant,
  size,
  asChild = false,
  ...props
}: React.ComponentProps<"button"> &
  VariantProps<typeof buttonVariants> & {
    asChild?: boolean
  }) {
  const Comp = asChild ? Slot : "button"
  return (
    <Comp
      data-slot="button"
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
    />
  )
}

export { Button, buttonVariants }
```

Install radix slot if not already present:

```bash
cd /home/jzy/projects/howmuchyousay/frontend
npm install @radix-ui/react-slot
```

- [ ] **Step 3: Add Input component**

Create `frontend/src/components/ui/input.tsx`:

```tsx
import * as React from "react"
import { cn } from "@/lib/utils"

function Input({ className, type, ...props }: React.ComponentProps<"input">) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn(
        "flex h-10 w-full rounded-base border-2 border-border bg-secondary-background px-3 py-2 text-sm font-base ring-offset-white selection:bg-main selection:text-main-foreground file:border-0 file:bg-transparent file:text-sm file:font-base placeholder:text-foreground/50 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-black focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50",
        className,
      )}
      {...props}
    />
  )
}

export { Input }
```

- [ ] **Step 4: Add Card component**

Create `frontend/src/components/ui/card.tsx`:

```tsx
import * as React from "react"
import { cn } from "@/lib/utils"

function Card({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="card"
      className={cn(
        "rounded-base border-2 border-border bg-secondary-background text-foreground shadow-shadow",
        className,
      )}
      {...props}
    />
  )
}

function CardHeader({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="card-header"
      className={cn("flex flex-col gap-1.5 p-6", className)}
      {...props}
    />
  )
}

function CardTitle({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="card-title"
      className={cn("text-lg font-heading leading-none", className)}
      {...props}
    />
  )
}

function CardDescription({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="card-description"
      className={cn("text-sm text-foreground/70", className)}
      {...props}
    />
  )
}

function CardContent({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="card-content"
      className={cn("p-6 pt-0", className)}
      {...props}
    />
  )
}

function CardFooter({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="card-footer"
      className={cn("flex items-center p-6 pt-0", className)}
      {...props}
    />
  )
}

export { Card, CardHeader, CardFooter, CardTitle, CardDescription, CardContent }
```

- [ ] **Step 5: Add Label component**

Create `frontend/src/components/ui/label.tsx`:

```tsx
import * as React from "react"
import { cn } from "@/lib/utils"

function Label({ className, ...props }: React.ComponentProps<"label">) {
  return (
    <label
      data-slot="label"
      className={cn(
        "text-sm font-heading leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70",
        className,
      )}
      {...props}
    />
  )
}

export { Label }
```

- [ ] **Step 6: Add Progress component**

Create `frontend/src/components/ui/progress.tsx`:

```tsx
import * as React from "react"
import { cn } from "@/lib/utils"

interface ProgressProps extends React.ComponentProps<"div"> {
  value?: number
  max?: number
}

function Progress({ className, value = 0, max = 100, ...props }: ProgressProps) {
  const percentage = Math.min(Math.max((value / max) * 100, 0), 100)

  return (
    <div
      data-slot="progress"
      role="progressbar"
      aria-valuenow={value}
      aria-valuemin={0}
      aria-valuemax={max}
      className={cn(
        "relative h-4 w-full overflow-hidden rounded-base border-2 border-border bg-secondary-background",
        className,
      )}
      {...props}
    >
      <div
        className="h-full bg-main transition-all"
        style={{ width: `${percentage}%` }}
      />
    </div>
  )
}

export { Progress }
```

- [ ] **Step 7: Add Badge component**

Create `frontend/src/components/ui/badge.tsx`:

```tsx
import { cva, type VariantProps } from "class-variance-authority"
import * as React from "react"
import { cn } from "@/lib/utils"

const badgeVariants = cva(
  "inline-flex items-center rounded-base border-2 border-border px-2.5 py-0.5 text-xs font-heading transition-colors",
  {
    variants: {
      variant: {
        default: "bg-main text-main-foreground",
        secondary: "bg-secondary-background text-foreground",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  },
)

function Badge({
  className,
  variant,
  ...props
}: React.ComponentProps<"div"> & VariantProps<typeof badgeVariants>) {
  return (
    <div
      data-slot="badge"
      className={cn(badgeVariants({ variant }), className)}
      {...props}
    />
  )
}

export { Badge, badgeVariants }
```

- [ ] **Step 8: Verify components render**

Temporarily update `frontend/src/App.tsx` to render all components:

```tsx
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card"
import { Label } from "@/components/ui/label"
import { Progress } from "@/components/ui/progress"
import { Badge } from "@/components/ui/badge"

export default function App() {
  return (
    <div className="min-h-screen bg-background flex items-center justify-center">
      <Card className="w-96">
        <CardHeader>
          <CardTitle>Component Test</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div>
            <Label htmlFor="test">Test Input</Label>
            <Input id="test" placeholder="Type here..." />
          </div>
          <Button>Default Button</Button>
          <Button variant="neutral">Neutral Button</Button>
          <Progress value={60} />
          <div className="flex gap-2">
            <Badge>Default</Badge>
            <Badge variant="secondary">Secondary</Badge>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
```

Run: `cd /home/jzy/projects/howmuchyousay/frontend && npm run dev`

Verify in browser:
- Card renders with black border and shadow
- Button has neobrutalism shadow that shifts on hover
- Input has border styling
- Progress bar shows 60% fill
- Badges show with border

- [ ] **Step 9: Commit**

```bash
cd /home/jzy/projects/howmuchyousay
git add frontend/src/components/ui/ frontend/src/lib/utils.ts frontend/components.json frontend/package.json frontend/package-lock.json
git commit -m "feat(frontend): add neobrutalism UI components (Button, Input, Card, Label, Progress, Badge)"
```

---

### Task 3: TypeScript types and API client

**Files:**
- Create: `frontend/src/types/index.ts`
- Create: `frontend/src/api/client.ts`
- Create: `frontend/src/api/gameApi.ts`

- [ ] **Step 1: Create shared TypeScript types**

Create `frontend/src/types/index.ts`:

```typescript
export type GameMode = "comparison" | "guess"
export type GameStatus = "crawling" | "ready" | "in_progress" | "finished" | "failed"

export interface CreateGameRequest {
  nick: string
  shop_url: string
  game_mode: GameMode
  skip_crawl: boolean
}

export interface CreateGameResponse {
  session_id: string
}

export interface SessionResponse {
  id: string
  status: GameStatus
  game_mode: GameMode
  rounds_total: number
  current_round: number
  error_message?: string
}

export interface ProductDTO {
  id: string
  name: string
  image_url: string
}

export interface RoundResponse {
  number: number
  type: string
  product_a: ProductDTO
  product_b?: ProductDTO
}

export interface AnswerResponse {
  is_correct: boolean
  points: number
  correct_answer: string
}

export interface PlayerScore {
  player_id: string
  nick: string
  rank: number
  total_points: number
  correct_count: number
  total_rounds: number
  best_round_score: number
}

export interface ResultsResponse {
  session_id: string
  rankings: PlayerScore[]
}

export interface RoundHistoryEntry {
  round_number: number
  product_a: ProductDTO
  product_b: ProductDTO
  selected_answer: "a" | "b"
  is_correct: boolean
  points: number
  correct_answer: string
}

export interface ApiError {
  status: number
  code: string
  message: string
  details?: Record<string, unknown>
}
```

- [ ] **Step 2: Create API client**

Create `frontend/src/api/client.ts`:

```typescript
import type { ApiError } from "@/types"

const BASE_URL = import.meta.env.VITE_API_URL ?? ""

export class ApiRequestError extends Error {
  status: number
  code: string
  details?: Record<string, unknown>

  constructor(err: ApiError) {
    super(err.message)
    this.name = "ApiRequestError"
    this.status = err.status
    this.code = err.code
    this.details = err.details
  }
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...options?.headers,
    },
  })

  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new ApiRequestError({
      status: res.status,
      code: body.code ?? "unknown",
      message: body.error ?? "Request failed",
      details: body,
    })
  }

  return res.json()
}

export function get<T>(path: string): Promise<T> {
  return request<T>(path)
}

export function post<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, {
    method: "POST",
    body: JSON.stringify(body),
  })
}
```

- [ ] **Step 3: Create TanStack Query hooks**

Create `frontend/src/api/gameApi.ts`:

```typescript
import { useMutation, useQuery } from "@tanstack/react-query"
import { get, post } from "./client"
import type {
  CreateGameRequest,
  CreateGameResponse,
  SessionResponse,
  RoundResponse,
  AnswerResponse,
  ResultsResponse,
} from "@/types"

export function useCreateGame() {
  return useMutation({
    mutationFn: (data: CreateGameRequest) =>
      post<CreateGameResponse>("/api/game", data),
  })
}

export function useSession(sessionId: string) {
  return useQuery({
    queryKey: ["session", sessionId],
    queryFn: () => get<SessionResponse>(`/api/game/${sessionId}`),
    staleTime: Infinity,
  })
}

export function useRound(sessionId: string, roundNumber: number) {
  return useQuery({
    queryKey: ["round", sessionId, roundNumber],
    queryFn: () =>
      get<RoundResponse>(`/api/game/${sessionId}/round/${roundNumber}`),
    staleTime: Infinity,
    enabled: roundNumber > 0,
  })
}

export function useSubmitAnswer(sessionId: string, roundNumber: number) {
  return useMutation({
    mutationFn: (answer: string) =>
      post<AnswerResponse>(
        `/api/game/${sessionId}/round/${roundNumber}/answer`,
        { answer },
      ),
  })
}

export function useResults(sessionId: string) {
  return useQuery({
    queryKey: ["results", sessionId],
    queryFn: () => get<ResultsResponse>(`/api/game/${sessionId}/results`),
    staleTime: Infinity,
  })
}
```

- [ ] **Step 4: Verify TypeScript compiles**

```bash
cd /home/jzy/projects/howmuchyousay/frontend
npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
cd /home/jzy/projects/howmuchyousay
git add frontend/src/types/ frontend/src/api/
git commit -m "feat(frontend): add TypeScript types and TanStack Query API hooks"
```

---

### Task 4: Router setup and page shells

**Files:**
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/main.tsx`
- Create: `frontend/src/pages/HomePage.tsx`
- Create: `frontend/src/pages/SetupPage.tsx`
- Create: `frontend/src/pages/GamePage.tsx`
- Create: `frontend/src/pages/ResultsPage.tsx`

- [ ] **Step 1: Set up main.tsx with providers**

Replace `frontend/src/main.tsx`:

```tsx
import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { BrowserRouter } from "react-router"
import App from "./App"
import "./index.css"

const queryClient = new QueryClient()

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
)
```

- [ ] **Step 2: Create HomePage**

Create `frontend/src/pages/HomePage.tsx`:

```tsx
import { Link } from "react-router"
import { Button } from "@/components/ui/button"

export default function HomePage() {
  return (
    <div className="min-h-screen bg-background flex flex-col items-center justify-center gap-8">
      <div className="text-center">
        <h1 className="text-6xl font-heading text-foreground">
          HowMuchYouSay
        </h1>
        <p className="mt-4 text-lg text-foreground/70">
          Guess which product costs more!
        </p>
      </div>
      <Button size="lg" asChild>
        <Link to="/play">Play</Link>
      </Button>
    </div>
  )
}
```

- [ ] **Step 3: Create SetupPage shell**

Create `frontend/src/pages/SetupPage.tsx`:

```tsx
export default function SetupPage() {
  return (
    <div className="min-h-screen bg-background flex items-center justify-center">
      <p className="text-foreground">Setup page — coming in Task 5</p>
    </div>
  )
}
```

- [ ] **Step 4: Create GamePage shell**

Create `frontend/src/pages/GamePage.tsx`:

```tsx
export default function GamePage() {
  return (
    <div className="min-h-screen bg-background flex items-center justify-center">
      <p className="text-foreground">Game page — coming in Task 6</p>
    </div>
  )
}
```

- [ ] **Step 5: Create ResultsPage shell**

Create `frontend/src/pages/ResultsPage.tsx`:

```tsx
export default function ResultsPage() {
  return (
    <div className="min-h-screen bg-background flex items-center justify-center">
      <p className="text-foreground">Results page — coming in Task 8</p>
    </div>
  )
}
```

- [ ] **Step 6: Wire up App.tsx with routes**

Replace `frontend/src/App.tsx`:

```tsx
import { Routes, Route } from "react-router"
import HomePage from "@/pages/HomePage"
import SetupPage from "@/pages/SetupPage"
import GamePage from "@/pages/GamePage"
import ResultsPage from "@/pages/ResultsPage"

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<HomePage />} />
      <Route path="/play" element={<SetupPage />} />
      <Route path="/game/:sessionId" element={<GamePage />} />
      <Route path="/game/:sessionId/results" element={<ResultsPage />} />
    </Routes>
  )
}
```

- [ ] **Step 7: Verify routing works**

```bash
cd /home/jzy/projects/howmuchyousay/frontend
npm run dev
```

In browser:
- `http://localhost:5173/` → shows HowMuchYouSay title and Play button
- Click "Play" → navigates to `/play`, shows setup placeholder
- Manually go to `http://localhost:5173/game/test-id` → shows game placeholder
- Manually go to `http://localhost:5173/game/test-id/results` → shows results placeholder

- [ ] **Step 8: Commit**

```bash
cd /home/jzy/projects/howmuchyousay
git add frontend/src/
git commit -m "feat(frontend): add React Router with 4 page routes (Home, Setup, Game, Results)"
```

---

### Task 5: GameSetupForm and SetupPage

**Files:**
- Create: `frontend/src/components/setup/GameSetupForm.tsx`
- Modify: `frontend/src/pages/SetupPage.tsx`

- [ ] **Step 1: Create GameSetupForm component**

Create `frontend/src/components/setup/GameSetupForm.tsx`:

```tsx
import { useState } from "react"
import { useNavigate } from "react-router"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Card, CardHeader, CardTitle, CardContent, CardFooter } from "@/components/ui/card"
import { useCreateGame } from "@/api/gameApi"
import { ApiRequestError } from "@/api/client"

export default function GameSetupForm() {
  const navigate = useNavigate()
  const createGame = useCreateGame()

  const [nick, setNick] = useState("")
  const [shopUrl, setShopUrl] = useState("")
  const [error, setError] = useState<string | null>(null)

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)

    if (!nick.trim() || nick.length > 32) {
      setError("Nick is required (1-32 characters)")
      return
    }

    try {
      new URL(shopUrl)
    } catch {
      setError("Please enter a valid URL")
      return
    }

    createGame.mutate(
      {
        nick: nick.trim(),
        shop_url: shopUrl.trim(),
        game_mode: "comparison",
        skip_crawl: true,
      },
      {
        onSuccess: (data) => {
          navigate(`/game/${data.session_id}`)
        },
        onError: (err) => {
          if (err instanceof ApiRequestError) {
            if (err.code === "not_enough_products") {
              setError(
                "Not enough products in this shop's database. Try a different shop URL.",
              )
            } else {
              setError(err.message)
            }
          } else {
            setError("Something went wrong. Please try again.")
          }
        },
      },
    )
  }

  return (
    <Card className="w-full max-w-md">
      <form onSubmit={handleSubmit}>
        <CardHeader>
          <CardTitle>New Game</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label htmlFor="nick">Your Nick</Label>
            <Input
              id="nick"
              placeholder="Enter your nickname"
              value={nick}
              onChange={(e) => setNick(e.target.value)}
              maxLength={32}
              required
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="shop-url">Shop URL</Label>
            <Input
              id="shop-url"
              placeholder="https://example-shop.com"
              type="url"
              value={shopUrl}
              onChange={(e) => setShopUrl(e.target.value)}
              required
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label>Game Mode</Label>
            <div className="flex h-10 w-full items-center rounded-base border-2 border-border bg-secondary-background px-3 text-sm">
              Comparison (Which is more expensive?)
            </div>
          </div>
          {error && (
            <p className="text-sm font-base text-red-600 border-2 border-red-600 rounded-base p-2 bg-red-50">
              {error}
            </p>
          )}
        </CardContent>
        <CardFooter>
          <Button
            type="submit"
            className="w-full"
            disabled={createGame.isPending}
          >
            {createGame.isPending ? "Starting..." : "Start Game"}
          </Button>
        </CardFooter>
      </form>
    </Card>
  )
}
```

- [ ] **Step 2: Wire SetupPage to use the form**

Replace `frontend/src/pages/SetupPage.tsx`:

```tsx
import { Link } from "react-router"
import GameSetupForm from "@/components/setup/GameSetupForm"

export default function SetupPage() {
  return (
    <div className="min-h-screen bg-background flex flex-col items-center justify-center gap-6 p-4">
      <Link to="/" className="text-3xl font-heading text-foreground hover:underline">
        HowMuchYouSay
      </Link>
      <GameSetupForm />
    </div>
  )
}
```

- [ ] **Step 3: Verify in browser**

Run the dev server. Navigate to `/play`.

Verify:
- Form renders with Nick input, Shop URL input, Game Mode display, and Start Game button
- Client-side validation shows error for empty nick or invalid URL
- Button shows "Starting..." while pending
- If backend is running with a shop that has products: form submits and redirects to `/game/:sessionId`
- If backend returns 409 not_enough_products: error message appears inline

- [ ] **Step 4: Commit**

```bash
cd /home/jzy/projects/howmuchyousay
git add frontend/src/components/setup/ frontend/src/pages/SetupPage.tsx
git commit -m "feat(frontend): add game setup form with validation and API integration"
```

---

### Task 6: Game components (ProductCard, ComparisonRound, RoundResult, RoundCounter, ScoreTracker)

**Files:**
- Create: `frontend/src/components/game/ProductCard.tsx`
- Create: `frontend/src/components/game/ComparisonRound.tsx`
- Create: `frontend/src/components/game/RoundResult.tsx`
- Create: `frontend/src/components/game/RoundCounter.tsx`
- Create: `frontend/src/components/game/ScoreTracker.tsx`

- [ ] **Step 1: Create ProductCard**

Create `frontend/src/components/game/ProductCard.tsx`:

```tsx
import { Card, CardContent } from "@/components/ui/card"
import { cn } from "@/lib/utils"
import type { ProductDTO } from "@/types"

interface ProductCardProps {
  product: ProductDTO
  label: string
  selected: boolean
  disabled: boolean
  isCorrectAnswer?: boolean
  isWrongPick?: boolean
  onClick: () => void
}

export default function ProductCard({
  product,
  label,
  selected,
  disabled,
  isCorrectAnswer,
  isWrongPick,
  onClick,
}: ProductCardProps) {
  return (
    <Card
      className={cn(
        "w-full max-w-xs cursor-pointer transition-all",
        selected && !isCorrectAnswer && !isWrongPick && "ring-4 ring-main translate-x-boxShadowX translate-y-boxShadowY shadow-none",
        isCorrectAnswer && "ring-4 ring-green-500 border-green-500",
        isWrongPick && "ring-4 ring-red-500 border-red-500 opacity-75",
        disabled && "cursor-default",
        !disabled && !selected && "hover:translate-x-boxShadowX hover:translate-y-boxShadowY hover:shadow-none",
      )}
      onClick={() => {
        if (!disabled) onClick()
      }}
    >
      <CardContent className="flex flex-col items-center gap-3 p-4">
        <span className="text-xs font-heading text-foreground/50 uppercase">
          {label}
        </span>
        <div className="w-full aspect-square overflow-hidden rounded-base border-2 border-border bg-background">
          <img
            src={product.image_url}
            alt={product.name}
            className="w-full h-full object-contain"
            onError={(e) => {
              e.currentTarget.src = "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='200' height='200'%3E%3Crect width='200' height='200' fill='%23e5e5e5'/%3E%3Ctext x='50%25' y='50%25' text-anchor='middle' dy='.3em' fill='%23999' font-size='14'%3ENo Image%3C/text%3E%3C/svg%3E"
            }}
          />
        </div>
        <p className="text-center text-sm font-heading leading-tight">
          {product.name}
        </p>
        {isCorrectAnswer && (
          <p className="text-sm font-heading text-green-600">
            ✓ More expensive
          </p>
        )}
        {isWrongPick && (
          <p className="text-sm font-heading text-red-600">
            ✗ Not this one
          </p>
        )}
      </CardContent>
    </Card>
  )
}
```

- [ ] **Step 2: Create RoundCounter**

Create `frontend/src/components/game/RoundCounter.tsx`:

```tsx
import { Progress } from "@/components/ui/progress"

interface RoundCounterProps {
  current: number
  total: number
}

export default function RoundCounter({ current, total }: RoundCounterProps) {
  return (
    <div className="w-full max-w-md flex flex-col gap-2">
      <span className="text-sm font-heading text-foreground">
        Round {current} / {total}
      </span>
      <Progress value={current} max={total} />
    </div>
  )
}
```

- [ ] **Step 3: Create ScoreTracker**

Create `frontend/src/components/game/ScoreTracker.tsx`:

```tsx
import { Badge } from "@/components/ui/badge"

interface ScoreTrackerProps {
  score: number
}

export default function ScoreTracker({ score }: ScoreTrackerProps) {
  return (
    <Badge variant="secondary" className="text-base px-4 py-1">
      Score: {score}
    </Badge>
  )
}
```

- [ ] **Step 4: Create RoundResult**

Create `frontend/src/components/game/RoundResult.tsx`:

```tsx
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"

interface RoundResultProps {
  isCorrect: boolean
  points: number
  isFinalRound: boolean
  onNext: () => void
}

export default function RoundResult({
  isCorrect,
  points,
  isFinalRound,
  onNext,
}: RoundResultProps) {
  return (
    <div className="flex flex-col items-center gap-4 mt-4">
      <Badge
        className={
          isCorrect
            ? "bg-green-400 text-black text-base px-4 py-2"
            : "bg-red-400 text-black text-base px-4 py-2"
        }
      >
        {isCorrect ? `Correct! +${points} points` : "Wrong! 0 points"}
      </Badge>
      <Button onClick={onNext}>
        {isFinalRound ? "See Results" : "Next Round"}
      </Button>
    </div>
  )
}
```

- [ ] **Step 5: Create ComparisonRound**

Create `frontend/src/components/game/ComparisonRound.tsx`:

Note: The backend's `correct_answer` for comparison mode is `"a"` or `"b"` (not prices). We use `isCorrectAnswer` / `isWrongPick` props on ProductCard to visually indicate the result.

```tsx
import { useState } from "react"
import { Button } from "@/components/ui/button"
import ProductCard from "./ProductCard"
import type { ProductDTO, AnswerResponse } from "@/types"

interface ComparisonRoundProps {
  productA: ProductDTO
  productB: ProductDTO
  onSubmit: (answer: "a" | "b") => void
  isSubmitting: boolean
  result: AnswerResponse | null
}

export default function ComparisonRound({
  productA,
  productB,
  onSubmit,
  isSubmitting,
  result,
}: ComparisonRoundProps) {
  const [selected, setSelected] = useState<"a" | "b" | null>(null)
  const answered = result !== null

  return (
    <div className="flex flex-col items-center gap-6">
      <h2 className="text-2xl font-heading text-center">
        Which product is more expensive?
      </h2>
      <div className="flex flex-col sm:flex-row gap-6 items-center sm:items-start">
        <ProductCard
          product={productA}
          label="Product A"
          selected={selected === "a"}
          disabled={answered}
          isCorrectAnswer={answered && result.correct_answer === "a"}
          isWrongPick={answered && selected === "a" && result.correct_answer !== "a"}
          onClick={() => setSelected("a")}
        />
        <div className="flex items-center text-2xl font-heading text-foreground/30 self-center">
          VS
        </div>
        <ProductCard
          product={productB}
          label="Product B"
          selected={selected === "b"}
          disabled={answered}
          isCorrectAnswer={answered && result.correct_answer === "b"}
          isWrongPick={answered && selected === "b" && result.correct_answer !== "b"}
          onClick={() => setSelected("b")}
        />
      </div>
      {!answered && (
        <Button
          size="lg"
          disabled={selected === null || isSubmitting}
          onClick={() => {
            if (selected) onSubmit(selected)
          }}
        >
          {isSubmitting ? "Submitting..." : "Lock In Answer"}
        </Button>
      )}
    </div>
  )
}
```

- [ ] **Step 6: Commit**

```bash
cd /home/jzy/projects/howmuchyousay
git add frontend/src/components/game/
git commit -m "feat(frontend): add game components (ProductCard, ComparisonRound, RoundResult, RoundCounter, ScoreTracker)"
```

---

### Task 7: GamePage — full gameplay loop

**Files:**
- Modify: `frontend/src/pages/GamePage.tsx`

- [ ] **Step 1: Implement GamePage with full round loop**

Replace `frontend/src/pages/GamePage.tsx`:

```tsx
import { useState, useEffect } from "react"
import { useParams, useNavigate, Link } from "react-router"
import { useSession, useRound, useSubmitAnswer } from "@/api/gameApi"
import ComparisonRound from "@/components/game/ComparisonRound"
import RoundResult from "@/components/game/RoundResult"
import RoundCounter from "@/components/game/RoundCounter"
import ScoreTracker from "@/components/game/ScoreTracker"
import type { AnswerResponse, RoundHistoryEntry } from "@/types"

export default function GamePage() {
  const { sessionId } = useParams<{ sessionId: string }>()
  const navigate = useNavigate()

  const { data: session, isLoading: sessionLoading, error: sessionError } = useSession(sessionId!)
  const [localRound, setLocalRound] = useState(0)
  const [totalScore, setTotalScore] = useState(0)
  const [roundPhase, setRoundPhase] = useState<"answering" | "result">("answering")
  const [currentResult, setCurrentResult] = useState<AnswerResponse | null>(null)
  const [roundHistory, setRoundHistory] = useState<RoundHistoryEntry[]>([])

  useEffect(() => {
    if (session && localRound === 0) {
      setLocalRound(session.current_round)
    }
  }, [session, localRound])

  const { data: round, isLoading: roundLoading } = useRound(sessionId!, localRound)
  const submitAnswer = useSubmitAnswer(sessionId!, localRound)

  function handleSubmitAnswer(answer: "a" | "b") {
    submitAnswer.mutate(answer, {
      onSuccess: (result) => {
        setCurrentResult(result)
        setTotalScore((prev) => prev + result.points)
        setRoundPhase("result")

        if (round && round.product_a && round.product_b) {
          setRoundHistory((prev) => [
            ...prev,
            {
              round_number: localRound,
              product_a: round.product_a,
              product_b: round.product_b!,
              selected_answer: answer,
              is_correct: result.is_correct,
              points: result.points,
              correct_answer: result.correct_answer,
            },
          ])
        }
      },
    })
  }

  function handleNextRound() {
    if (session && localRound >= session.rounds_total) {
      navigate(`/game/${sessionId}/results`, {
        state: { roundHistory, totalScore: totalScore },
      })
      return
    }
    setLocalRound((prev) => prev + 1)
    setRoundPhase("answering")
    setCurrentResult(null)
  }

  if (!sessionId) {
    navigate("/")
    return null
  }

  if (sessionLoading) {
    return (
      <div className="min-h-screen bg-background flex items-center justify-center">
        <p className="text-foreground font-heading">Loading game...</p>
      </div>
    )
  }

  if (sessionError || !session) {
    return (
      <div className="min-h-screen bg-background flex flex-col items-center justify-center gap-4">
        <p className="text-foreground font-heading">Game not found</p>
        <Link to="/" className="text-main underline font-heading">
          Back to Home
        </Link>
      </div>
    )
  }

  if (session.status === "finished") {
    navigate(`/game/${sessionId}/results`)
    return null
  }

  if (session.status === "failed") {
    return (
      <div className="min-h-screen bg-background flex flex-col items-center justify-center gap-4">
        <p className="text-foreground font-heading">Game failed</p>
        <p className="text-foreground/70">{session.error_message}</p>
        <Link to="/play" className="text-main underline font-heading">
          Try Again
        </Link>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-background flex flex-col items-center p-4 gap-6">
      <Link to="/" className="text-2xl font-heading text-foreground hover:underline">
        HowMuchYouSay
      </Link>
      <div className="flex items-center gap-6 w-full max-w-md justify-between">
        <RoundCounter
          current={localRound}
          total={session.rounds_total}
        />
        <ScoreTracker score={totalScore} />
      </div>

      {roundLoading ? (
        <p className="text-foreground font-heading mt-8">Loading round...</p>
      ) : round && round.product_a && round.product_b ? (
        <>
          <ComparisonRound
            key={localRound}
            productA={round.product_a}
            productB={round.product_b}
            onSubmit={handleSubmitAnswer}
            isSubmitting={submitAnswer.isPending}
            result={roundPhase === "result" ? currentResult : null}
          />
          {roundPhase === "result" && currentResult && (
            <RoundResult
              isCorrect={currentResult.is_correct}
              points={currentResult.points}
              isFinalRound={localRound >= session.rounds_total}
              onNext={handleNextRound}
            />
          )}
        </>
      ) : (
        <p className="text-foreground font-heading mt-8">
          Waiting for round data...
        </p>
      )}
    </div>
  )
}
```

- [ ] **Step 2: Verify gameplay loop in browser**

Prerequisites: backend running, a shop with 20+ products in the database.

1. Start the backend: `cd /home/jzy/projects/howmuchyousay && make dev`
2. Start the frontend: `cd /home/jzy/projects/howmuchyousay/frontend && npm run dev`
3. Go to `http://localhost:5173/play`
4. Enter a nick, shop URL that has products, submit

Verify:
- Redirects to `/game/:sessionId`
- Round 1 loads with two product cards
- Clicking a card selects it (visual highlight)
- Clicking "Lock In Answer" submits the answer
- Result badge shows correct/incorrect with points
- "Next Round" advances to round 2
- Score updates cumulatively
- Progress bar advances
- After round 10, "See Results" redirects to results page

- [ ] **Step 3: Commit**

```bash
cd /home/jzy/projects/howmuchyousay
git add frontend/src/pages/GamePage.tsx
git commit -m "feat(frontend): implement full gameplay loop in GamePage"
```

---

### Task 8: ResultsPage and GameSummary

**Files:**
- Create: `frontend/src/components/results/GameSummary.tsx`
- Modify: `frontend/src/pages/ResultsPage.tsx`

- [ ] **Step 1: Create GameSummary component**

Create `frontend/src/components/results/GameSummary.tsx`:

```tsx
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import type { RoundHistoryEntry } from "@/types"

interface GameSummaryProps {
  totalScore: number
  roundHistory: RoundHistoryEntry[]
}

export default function GameSummary({ totalScore, roundHistory }: GameSummaryProps) {
  const correctCount = roundHistory.filter((r) => r.is_correct).length

  return (
    <Card className="w-full max-w-2xl">
      <CardHeader className="text-center">
        <CardTitle className="text-3xl">Game Over!</CardTitle>
        <div className="flex justify-center gap-4 mt-2">
          <Badge className="text-xl px-6 py-2">
            Score: {totalScore}
          </Badge>
          <Badge variant="secondary" className="text-xl px-6 py-2">
            {correctCount} / {roundHistory.length} correct
          </Badge>
        </div>
      </CardHeader>
      <CardContent>
        <div className="flex flex-col gap-3">
          {roundHistory.map((entry) => (
            <div
              key={entry.round_number}
              className="flex items-center gap-3 p-3 rounded-base border-2 border-border bg-background"
            >
              <span className="text-sm font-heading w-20 shrink-0">
                Round {entry.round_number}
              </span>
              <span className="text-sm flex-1 truncate">
                {entry.product_a.name} vs {entry.product_b.name}
              </span>
              <Badge
                className={
                  entry.is_correct
                    ? "bg-green-400 text-black"
                    : "bg-red-400 text-black"
                }
              >
                {entry.is_correct ? `+${entry.points}` : "0"}
              </Badge>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  )
}
```

- [ ] **Step 2: Implement ResultsPage**

Replace `frontend/src/pages/ResultsPage.tsx`:

```tsx
import { Link, useParams, useLocation } from "react-router"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { useResults } from "@/api/gameApi"
import GameSummary from "@/components/results/GameSummary"
import type { RoundHistoryEntry } from "@/types"

interface LocationState {
  roundHistory: RoundHistoryEntry[]
  totalScore: number
}

export default function ResultsPage() {
  const { sessionId } = useParams<{ sessionId: string }>()
  const location = useLocation()
  const state = location.state as LocationState | null

  const { data: results, isLoading } = useResults(sessionId!)

  if (state && state.roundHistory.length > 0) {
    return (
      <div className="min-h-screen bg-background flex flex-col items-center p-4 gap-6">
        <Link to="/" className="text-2xl font-heading text-foreground hover:underline">
          HowMuchYouSay
        </Link>
        <GameSummary
          totalScore={state.totalScore}
          roundHistory={state.roundHistory}
        />
        <Button size="lg" asChild>
          <Link to="/play">Play Again</Link>
        </Button>
      </div>
    )
  }

  if (isLoading) {
    return (
      <div className="min-h-screen bg-background flex items-center justify-center">
        <p className="text-foreground font-heading">Loading results...</p>
      </div>
    )
  }

  if (!results) {
    return (
      <div className="min-h-screen bg-background flex flex-col items-center justify-center gap-4">
        <p className="text-foreground font-heading">Results not found</p>
        <Link to="/" className="text-main underline font-heading">
          Back to Home
        </Link>
      </div>
    )
  }

  const player = results.rankings[0]

  return (
    <div className="min-h-screen bg-background flex flex-col items-center p-4 gap-6">
      <Link to="/" className="text-2xl font-heading text-foreground hover:underline">
        HowMuchYouSay
      </Link>
      <div className="text-center">
        <h1 className="text-3xl font-heading">Game Over!</h1>
        {player && (
          <div className="flex justify-center gap-4 mt-4">
            <Badge className="text-xl px-6 py-2">
              Score: {player.total_points}
            </Badge>
            <Badge variant="secondary" className="text-xl px-6 py-2">
              {player.correct_count} / {player.total_rounds} correct
            </Badge>
          </div>
        )}
      </div>
      <Button size="lg" asChild>
        <Link to="/play">Play Again</Link>
      </Button>
    </div>
  )
}
```

- [ ] **Step 3: Verify results page**

Play through a full game and verify:
- After round 10, clicking "See Results" navigates to results page
- Full round-by-round breakdown is shown with green/red badges
- Total score and correct count displayed
- "Play Again" navigates to `/play`
- Direct access to `/game/:id/results` (refresh) shows server-side results (score, correct count) without round breakdown

- [ ] **Step 4: Commit**

```bash
cd /home/jzy/projects/howmuchyousay
git add frontend/src/components/results/ frontend/src/pages/ResultsPage.tsx
git commit -m "feat(frontend): add results page with game summary and round breakdown"
```

---

### Task 9: Add CORS to backend for dev, update Makefile

**Files:**
- Modify: `backend/internal/server/server.go`
- Modify: `Makefile`

- [ ] **Step 1: Add CORS middleware to backend**

The Vite proxy handles CORS in dev, but for direct API calls (e.g., from a different port), add CORS headers. Modify `backend/internal/server/server.go` to add a CORS middleware:

```go
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
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
```

- [ ] **Step 2: Add frontend dev command to Makefile**

Add to `Makefile`:

```makefile
frontend-dev:
	cd frontend && npm run dev

frontend-install:
	cd frontend && npm install
```

- [ ] **Step 3: Verify full end-to-end flow**

1. `make dev` (starts backend + postgres)
2. `make frontend-dev` (starts Vite dev server)
3. Play through a complete game from `/` → `/play` → `/game/:id` → `/game/:id/results`

Verify:
- Correct answers show green ring on the right product
- Wrong answers show red ring on your pick + green on the correct one
- Round breakdown on results page shows all 10 rounds
- "Play Again" works

- [ ] **Step 4: Commit**

```bash
cd /home/jzy/projects/howmuchyousay
git add backend/internal/server/server.go Makefile
git commit -m "feat: add CORS support and frontend Makefile targets"
```

---

### Task 10: Final polish and cleanup

**Files:**
- Modify: `frontend/src/App.tsx` (add catch-all route)
- Modify: `frontend/index.html` (title)

- [ ] **Step 1: Update page title**

In `frontend/index.html`, change the `<title>` tag:

```html
<title>HowMuchYouSay</title>
```

- [ ] **Step 2: Add catch-all route for 404**

Update `frontend/src/App.tsx`:

```tsx
import { Routes, Route, Link } from "react-router"
import HomePage from "@/pages/HomePage"
import SetupPage from "@/pages/SetupPage"
import GamePage from "@/pages/GamePage"
import ResultsPage from "@/pages/ResultsPage"
import { Button } from "@/components/ui/button"

function NotFound() {
  return (
    <div className="min-h-screen bg-background flex flex-col items-center justify-center gap-4">
      <h1 className="text-4xl font-heading">404</h1>
      <p className="text-foreground/70">Page not found</p>
      <Button asChild>
        <Link to="/">Back to Home</Link>
      </Button>
    </div>
  )
}

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<HomePage />} />
      <Route path="/play" element={<SetupPage />} />
      <Route path="/game/:sessionId" element={<GamePage />} />
      <Route path="/game/:sessionId/results" element={<ResultsPage />} />
      <Route path="*" element={<NotFound />} />
    </Routes>
  )
}
```

- [ ] **Step 3: Verify TypeScript compiles cleanly**

```bash
cd /home/jzy/projects/howmuchyousay/frontend
npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 4: Verify build succeeds**

```bash
cd /home/jzy/projects/howmuchyousay/frontend
npm run build
```

Expected: build completes without errors, output in `frontend/dist/`.

- [ ] **Step 5: Full end-to-end smoke test**

Run through the complete flow one final time:
1. Home page loads → click Play
2. Enter nick + shop URL → Start Game
3. Play through 10 rounds — select products, submit answers, see results
4. Results page shows score + breakdown → click Play Again
5. Navigate to a random URL → 404 page shows
6. Refresh during a game → game resumes

- [ ] **Step 6: Commit**

```bash
cd /home/jzy/projects/howmuchyousay
git add frontend/
git commit -m "feat(frontend): add 404 page, update title, final polish"
```
