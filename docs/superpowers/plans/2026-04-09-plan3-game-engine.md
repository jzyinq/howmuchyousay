# Plan 3: Game Engine

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the pure game logic layer: scoring calculations for both game modes, answer evaluation, round generation from product pools, and results aggregation with rankings.

**Architecture:** The game package (`internal/game/`) is a stateless function library with zero database dependencies. Functions accept model structs and primitives, return results. No interfaces, no state. The API layer (Plan 4) reads from stores, calls game functions, writes results back. Round generation uses injected `*rand.Rand` for deterministic testing. All scoring formulas come from the project spec (`docs/superpowers/specs/2026-04-09-howmuchyousay-design.md`).

**Tech Stack:** Go 1.23+, `math` (Abs), `math/rand/v2`, `sort`, `fmt`, `github.com/google/uuid`, existing `internal/models` from Plan 1, `github.com/stretchr/testify/assert` + `require` for tests

**Plan sequence:** This is Plan 3 of 5:
1. Database, Models & Store Layer (done)
2. Crawler CLI & AI Agent (in progress, separate branch)
3. **Game Engine** (this plan)
4. Single Player (API + Frontend)
5. Multiplayer (WebSocket + Frontend)

**Prerequisite:** Plan 1 must be fully implemented (models package with Product, Round, Answer, Player, GameSession types).

---

## File Structure

```
backend/internal/game/
├── scoring.go          # Price difference %, deviation %, point tier mapping
├── scoring_test.go     # Table-driven tests for all scoring tiers and edge cases
├── comparison.go       # Evaluate comparison answers ("a"/"b" -> correct/incorrect + points)
├── comparison_test.go  # Tests for comparison evaluation
├── guess.go            # Evaluate guess answers (guessed price -> deviation -> points)
├── guess_test.go       # Tests for guess evaluation
├── round_gen.go        # Generate RoundDefs from a product pool (comparison pairs / guess singles)
├── round_gen_test.go   # Deterministic tests with seeded RNG
├── results.go          # Aggregate player scores, build sorted rankings
└── results_test.go     # Tests for results aggregation
```

Each file has a single responsibility:
- **scoring.go** — Pure math: price difference percentage, guess deviation percentage, point tier lookup. Used by both comparison.go and guess.go.
- **comparison.go** — Takes an answer string ("a"/"b"), product prices, determines correctness and points. Delegates to scoring.go for tier calculation.
- **guess.go** — Takes a guessed price and actual price, determines correctness and points. Delegates to scoring.go for deviation calculation.
- **round_gen.go** — Selects products from a pool to create `RoundDef` structs (intermediate before DB persist). Handles constraints: no product reuse, minimum 5% price difference for comparison pairs.
- **results.go** — Given all players, answers, and rounds for a session, produces a ranked list of `PlayerScore` structs with statistics.

---

### Task 1: Scoring Functions

**Files:**
- Create: `backend/internal/game/scoring.go`
- Create: `backend/internal/game/scoring_test.go`

This task implements the core math used by both game modes. All formulas come from the spec.

**Comparison scoring:**
- Price difference: `|price_a - price_b| / min(price_a, price_b) * 100`
- Tiers: >50% → 1pt (easy), 20-50% → 2pts (medium), 5-20% → 3pts (hard)
- Minimum difference enforced at round generation, not here

**Guess scoring:**
- Deviation: `|guessed - actual| / actual * 100`
- Tiers: <=2% → 5pts (exact hit), <=10% → 3pts, <=20% → 2pts, <=30% → 1pt, >30% → 0pts

- [ ] **Step 1: Write scoring_test.go with table-driven tests**

Create `backend/internal/game/scoring_test.go`:

```go
package game

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPriceDiffPercent(t *testing.T) {
	tests := []struct {
		name     string
		priceA   float64
		priceB   float64
		expected float64
	}{
		{"equal prices", 100.0, 100.0, 0.0},
		{"50% difference (A > B)", 150.0, 100.0, 50.0},
		{"50% difference (B > A)", 100.0, 150.0, 50.0},
		{"100% difference", 200.0, 100.0, 100.0},
		{"small difference", 105.0, 100.0, 5.0},
		{"large difference", 500.0, 100.0, 400.0},
		{"decimal prices", 29.99, 19.99, 50.025012506253126},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PriceDiffPercent(tt.priceA, tt.priceB)
			assert.InDelta(t, tt.expected, result, 0.001)
		})
	}
}

func TestComparisonPoints(t *testing.T) {
	tests := []struct {
		name        string
		diffPercent float64
		expected    int
	}{
		{"easy: > 50%", 60.0, 1},
		{"easy: exactly 50.01%", 50.01, 1},
		{"medium: exactly 50%", 50.0, 2},
		{"medium: 35%", 35.0, 2},
		{"medium: exactly 20%", 20.0, 2},
		{"hard: 19.99%", 19.99, 3},
		{"hard: 10%", 10.0, 3},
		{"hard: exactly 5%", 5.0, 3},
		{"hard: 5.01%", 5.01, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ComparisonPoints(tt.diffPercent)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGuessDeviation(t *testing.T) {
	tests := []struct {
		name     string
		guessed  float64
		actual   float64
		expected float64
	}{
		{"exact match", 100.0, 100.0, 0.0},
		{"10% over", 110.0, 100.0, 10.0},
		{"10% under", 90.0, 100.0, 10.0},
		{"50% over", 150.0, 100.0, 50.0},
		{"50% under", 50.0, 100.0, 50.0},
		{"2% deviation", 102.0, 100.0, 2.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GuessDeviation(tt.guessed, tt.actual)
			assert.InDelta(t, tt.expected, result, 0.001)
		})
	}
}

func TestGuessPoints(t *testing.T) {
	tests := []struct {
		name             string
		deviationPercent float64
		expected         int
	}{
		{"exact hit: 0%", 0.0, 5},
		{"exact hit: 1%", 1.0, 5},
		{"exact hit: 2%", 2.0, 5},
		{"close: 2.01%", 2.01, 3},
		{"close: 5%", 5.0, 3},
		{"close: 10%", 10.0, 3},
		{"decent: 10.01%", 10.01, 2},
		{"decent: 15%", 15.0, 2},
		{"decent: 20%", 20.0, 2},
		{"far: 20.01%", 20.01, 1},
		{"far: 25%", 25.0, 1},
		{"far: 30%", 30.0, 1},
		{"miss: 30.01%", 30.01, 0},
		{"miss: 50%", 50.0, 0},
		{"miss: 100%", 100.0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GuessPoints(tt.deviationPercent)
			assert.Equal(t, tt.expected, result)
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/game/ -v -count=1`
Working directory: `backend/`
Expected: Compilation error — package `game` does not exist yet.

- [ ] **Step 3: Implement scoring.go**

Create `backend/internal/game/scoring.go`:

```go
package game

import "math"

// PriceDiffPercent calculates the percentage price difference between two products.
// Formula: |price_a - price_b| / min(price_a, price_b) * 100
func PriceDiffPercent(priceA, priceB float64) float64 {
	diff := math.Abs(priceA - priceB)
	minPrice := math.Min(priceA, priceB)
	if minPrice == 0 {
		return 0
	}
	return diff / minPrice * 100
}

// ComparisonPoints returns the points for a comparison round based on price difference.
//
// Tiers (from spec):
//   - Difference > 50%:  1 point  (easy)
//   - Difference 20-50%: 2 points (medium)
//   - Difference 5-20%:  3 points (hard)
func ComparisonPoints(diffPercent float64) int {
	switch {
	case diffPercent > 50:
		return 1
	case diffPercent >= 20:
		return 2
	default:
		return 3
	}
}

// GuessDeviation calculates the percentage deviation of a guessed price from the actual price.
// Formula: |guessed - actual| / actual * 100
func GuessDeviation(guessed, actual float64) float64 {
	if actual == 0 {
		return 0
	}
	return math.Abs(guessed-actual) / actual * 100
}

// GuessPoints returns the points for a guess round based on deviation percentage.
//
// Tiers (from spec):
//   - Deviation <= 2%:  5 points (exact hit bonus)
//   - Deviation <= 10%: 3 points
//   - Deviation <= 20%: 2 points
//   - Deviation <= 30%: 1 point
//   - Deviation > 30%:  0 points
func GuessPoints(deviationPercent float64) int {
	switch {
	case deviationPercent <= 2:
		return 5
	case deviationPercent <= 10:
		return 3
	case deviationPercent <= 20:
		return 2
	case deviationPercent <= 30:
		return 1
	default:
		return 0
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/game/ -v -count=1`
Working directory: `backend/`
Expected: All tests PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/game/scoring.go backend/internal/game/scoring_test.go
git commit -m "feat(game): add scoring functions for comparison and guess modes"
```

---

### Task 2: Comparison Answer Evaluation

**Files:**
- Create: `backend/internal/game/comparison.go`
- Create: `backend/internal/game/comparison_test.go`

Evaluates a player's comparison answer. The player answers "a" or "b" indicating which product they think is more expensive (or cheaper — the question direction is implicit in `correctAnswer` stored on the round). This function checks correctness and calculates points.

- [ ] **Step 1: Write comparison_test.go**

Create `backend/internal/game/comparison_test.go`:

```go
package game

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEvalComparison(t *testing.T) {
	tests := []struct {
		name            string
		answer          string
		correctAnswer   string
		priceA          float64
		priceB          float64
		expectedCorrect bool
		expectedPoints  int
	}{
		{
			name:            "correct easy (>50% diff)",
			answer:          "a",
			correctAnswer:   "a",
			priceA:          200.0,
			priceB:          100.0,
			expectedCorrect: true,
			expectedPoints:  1,
		},
		{
			name:            "incorrect easy",
			answer:          "b",
			correctAnswer:   "a",
			priceA:          200.0,
			priceB:          100.0,
			expectedCorrect: false,
			expectedPoints:  0,
		},
		{
			name:            "correct medium (20-50% diff)",
			answer:          "b",
			correctAnswer:   "b",
			priceA:          100.0,
			priceB:          130.0,
			expectedCorrect: true,
			expectedPoints:  2,
		},
		{
			name:            "correct hard (5-20% diff)",
			answer:          "a",
			correctAnswer:   "a",
			priceA:          110.0,
			priceB:          100.0,
			expectedCorrect: true,
			expectedPoints:  3,
		},
		{
			name:            "incorrect hard",
			answer:          "b",
			correctAnswer:   "a",
			priceA:          110.0,
			priceB:          100.0,
			expectedCorrect: false,
			expectedPoints:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isCorrect, points := EvalComparison(tt.answer, tt.correctAnswer, tt.priceA, tt.priceB)
			assert.Equal(t, tt.expectedCorrect, isCorrect)
			assert.Equal(t, tt.expectedPoints, points)
		})
	}
}

func TestEvalComparisonInvalidAnswer(t *testing.T) {
	// Invalid answers are always incorrect with 0 points
	isCorrect, points := EvalComparison("c", "a", 200.0, 100.0)
	assert.False(t, isCorrect)
	assert.Equal(t, 0, points)

	isCorrect, points = EvalComparison("", "a", 200.0, 100.0)
	assert.False(t, isCorrect)
	assert.Equal(t, 0, points)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/game/ -v -run TestEvalComparison -count=1`
Working directory: `backend/`
Expected: Compilation error — `EvalComparison` undefined.

- [ ] **Step 3: Implement comparison.go**

Create `backend/internal/game/comparison.go`:

```go
package game

// EvalComparison evaluates a player's answer in a comparison round.
//
// Parameters:
//   - answer: the player's choice ("a" or "b")
//   - correctAnswer: the correct choice ("a" or "b"), pre-computed at round generation
//   - priceA, priceB: the prices of product A and product B
//
// Returns:
//   - isCorrect: whether the player chose the correct product
//   - points: points earned (0 if incorrect, 1/2/3 if correct based on difficulty)
func EvalComparison(answer, correctAnswer string, priceA, priceB float64) (bool, int) {
	if answer != "a" && answer != "b" {
		return false, 0
	}

	isCorrect := answer == correctAnswer
	if !isCorrect {
		return false, 0
	}

	diffPercent := PriceDiffPercent(priceA, priceB)
	points := ComparisonPoints(diffPercent)
	return true, points
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/game/ -v -run TestEvalComparison -count=1`
Working directory: `backend/`
Expected: All tests PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/game/comparison.go backend/internal/game/comparison_test.go
git commit -m "feat(game): add comparison answer evaluation"
```

---

### Task 3: Guess Answer Evaluation

**Files:**
- Create: `backend/internal/game/guess.go`
- Create: `backend/internal/game/guess_test.go`

Evaluates a player's guess. The player enters a price, we compute deviation from the actual price and return points.

- [ ] **Step 1: Write guess_test.go**

Create `backend/internal/game/guess_test.go`:

```go
package game

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEvalGuess(t *testing.T) {
	tests := []struct {
		name            string
		guessedPrice    float64
		actualPrice     float64
		expectedCorrect bool
		expectedPoints  int
	}{
		{
			name:            "exact match (0% deviation)",
			guessedPrice:    99.99,
			actualPrice:     99.99,
			expectedCorrect: true,
			expectedPoints:  5,
		},
		{
			name:            "within 2% (exact hit bonus)",
			guessedPrice:    101.0,
			actualPrice:     100.0,
			expectedCorrect: true,
			expectedPoints:  5,
		},
		{
			name:            "within 10%",
			guessedPrice:    92.0,
			actualPrice:     100.0,
			expectedCorrect: true,
			expectedPoints:  3,
		},
		{
			name:            "within 20%",
			guessedPrice:    85.0,
			actualPrice:     100.0,
			expectedCorrect: true,
			expectedPoints:  2,
		},
		{
			name:            "within 30%",
			guessedPrice:    75.0,
			actualPrice:     100.0,
			expectedCorrect: true,
			expectedPoints:  1,
		},
		{
			name:            "over 30% deviation",
			guessedPrice:    50.0,
			actualPrice:     100.0,
			expectedCorrect: false,
			expectedPoints:  0,
		},
		{
			name:            "way over",
			guessedPrice:    200.0,
			actualPrice:     100.0,
			expectedCorrect: false,
			expectedPoints:  0,
		},
		{
			name:            "boundary: exactly 2%",
			guessedPrice:    102.0,
			actualPrice:     100.0,
			expectedCorrect: true,
			expectedPoints:  5,
		},
		{
			name:            "boundary: exactly 10%",
			guessedPrice:    110.0,
			actualPrice:     100.0,
			expectedCorrect: true,
			expectedPoints:  3,
		},
		{
			name:            "boundary: exactly 30%",
			guessedPrice:    130.0,
			actualPrice:     100.0,
			expectedCorrect: true,
			expectedPoints:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isCorrect, points := EvalGuess(tt.guessedPrice, tt.actualPrice)
			assert.Equal(t, tt.expectedCorrect, isCorrect)
			assert.Equal(t, tt.expectedPoints, points)
		})
	}
}

func TestEvalGuessZeroPrice(t *testing.T) {
	// Actual price of 0 should not panic (edge case)
	isCorrect, points := EvalGuess(50.0, 0.0)
	assert.False(t, isCorrect)
	assert.Equal(t, 0, points)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/game/ -v -run TestEvalGuess -count=1`
Working directory: `backend/`
Expected: Compilation error — `EvalGuess` undefined.

- [ ] **Step 3: Implement guess.go**

Create `backend/internal/game/guess.go`:

```go
package game

import "fmt"

// EvalGuess evaluates a player's guess in a guess round.
//
// Parameters:
//   - guessedPrice: the price the player guessed
//   - actualPrice: the real price of the product
//
// Returns:
//   - isCorrect: true if the player earned any points (deviation <= 30%)
//   - points: points earned (0/1/2/3/5 based on deviation tiers)
func EvalGuess(guessedPrice, actualPrice float64) (bool, int) {
	if actualPrice <= 0 {
		return false, 0
	}

	deviation := GuessDeviation(guessedPrice, actualPrice)
	points := GuessPoints(deviation)
	return points > 0, points
}

// FormatCorrectGuessAnswer formats the actual price as a string for storage in Round.CorrectAnswer.
// Used during round generation to store the answer in a consistent format.
func FormatCorrectGuessAnswer(price float64) string {
	return fmt.Sprintf("%.2f", price)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/game/ -v -run TestEvalGuess -count=1`
Working directory: `backend/`
Expected: All tests PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/game/guess.go backend/internal/game/guess_test.go
git commit -m "feat(game): add guess answer evaluation"
```

---

### Task 4: Round Generation — Comparison Mode

**Files:**
- Create: `backend/internal/game/round_gen.go`
- Create: `backend/internal/game/round_gen_test.go`

Generates comparison rounds from a product pool. Each round picks a pair of products with >= 5% price difference. No product is reused across rounds. The correct answer is which product is more expensive ("a" or "b").

- [ ] **Step 1: Write round_gen.go with the RoundDef type and GenerateComparisonRounds stub**

Create `backend/internal/game/round_gen.go`:

```go
package game

import (
	"errors"

	"github.com/jzy/howmuchyousay/internal/models"
)

// MinPriceDiffPercent is the minimum price difference required between products in a comparison pair.
const MinPriceDiffPercent = 5.0

var (
	// ErrNotEnoughProducts is returned when the product pool is too small to generate the requested rounds.
	ErrNotEnoughProducts = errors.New("not enough products to generate rounds")
	// ErrNotEnoughValidPairs is returned when not enough product pairs meet the minimum price difference.
	ErrNotEnoughValidPairs = errors.New("not enough valid product pairs with sufficient price difference")
)

// RoundDef is the intermediate round definition before persisting to the database.
// It contains all data needed to create a Round row via RoundStore.Create().
type RoundDef struct {
	RoundNumber     int
	RoundType       models.RoundType
	ProductA        models.Product
	ProductB        *models.Product // nil for guess rounds
	CorrectAnswer   string
	DifficultyScore int
}
```

This step only defines the types and error sentinels. Note: `math/rand/v2` is NOT imported here — it will be added in Step 4 when `GenerateComparisonRounds` is implemented. Go rejects unused imports, so we only add it when needed.

- [ ] **Step 2: Write round_gen_test.go with comparison round generation tests**

Create `backend/internal/game/round_gen_test.go`:

```go
package game

import (
	"math/rand/v2"
	"testing"

	"github.com/google/uuid"
	"github.com/jzy/howmuchyousay/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeProduct creates a test product with the given name and price.
func makeProduct(name string, price float64) models.Product {
	return models.Product{
		ID:    uuid.New(),
		Name:  name,
		Price: price,
	}
}

func TestGenerateComparisonRounds(t *testing.T) {
	rng := rand.New(rand.NewPCG(42, 0))

	products := []models.Product{
		makeProduct("Product A", 100.0),
		makeProduct("Product B", 200.0),
		makeProduct("Product C", 50.0),
		makeProduct("Product D", 300.0),
		makeProduct("Product E", 150.0),
		makeProduct("Product F", 75.0),
		makeProduct("Product G", 250.0),
		makeProduct("Product H", 120.0),
		makeProduct("Product I", 180.0),
		makeProduct("Product J", 90.0),
	}

	rounds, err := GenerateComparisonRounds(products, 5, rng)
	require.NoError(t, err)
	assert.Len(t, rounds, 5)

	// Check each round
	usedProductIDs := make(map[uuid.UUID]bool)
	for i, r := range rounds {
		// Round numbers are 1-indexed
		assert.Equal(t, i+1, r.RoundNumber)
		// Round type is comparison
		assert.Equal(t, models.RoundTypeComparison, r.RoundType)
		// Product B is set for comparison rounds
		require.NotNil(t, r.ProductB)
		// Products are different
		assert.NotEqual(t, r.ProductA.ID, r.ProductB.ID)
		// Correct answer is "a" or "b"
		assert.Contains(t, []string{"a", "b"}, r.CorrectAnswer)
		// Difficulty score is 1, 2, or 3
		assert.Contains(t, []int{1, 2, 3}, r.DifficultyScore)
		// Price difference is >= 5%
		diff := PriceDiffPercent(r.ProductA.Price, r.ProductB.Price)
		assert.GreaterOrEqual(t, diff, MinPriceDiffPercent)
		// No product reuse
		assert.False(t, usedProductIDs[r.ProductA.ID], "product A reused in round %d", i+1)
		assert.False(t, usedProductIDs[r.ProductB.ID], "product B reused in round %d", i+1)
		usedProductIDs[r.ProductA.ID] = true
		usedProductIDs[r.ProductB.ID] = true
	}
}

func TestGenerateComparisonRoundsCorrectAnswer(t *testing.T) {
	rng := rand.New(rand.NewPCG(42, 0))

	products := []models.Product{
		makeProduct("Cheap", 10.0),
		makeProduct("Expensive", 100.0),
		makeProduct("Mid", 50.0),
		makeProduct("High", 200.0),
	}

	rounds, err := GenerateComparisonRounds(products, 2, rng)
	require.NoError(t, err)

	for _, r := range rounds {
		require.NotNil(t, r.ProductB)
		// Verify the correct answer points to the more expensive product
		if r.CorrectAnswer == "a" {
			assert.Greater(t, r.ProductA.Price, r.ProductB.Price)
		} else {
			assert.Greater(t, r.ProductB.Price, r.ProductA.Price)
		}
	}
}

func TestGenerateComparisonRoundsNotEnoughProducts(t *testing.T) {
	rng := rand.New(rand.NewPCG(42, 0))

	// Need 5 rounds = 10 products, only have 4
	products := []models.Product{
		makeProduct("A", 100.0),
		makeProduct("B", 200.0),
		makeProduct("C", 50.0),
		makeProduct("D", 300.0),
	}

	_, err := GenerateComparisonRounds(products, 5, rng)
	assert.ErrorIs(t, err, ErrNotEnoughProducts)
}

func TestGenerateComparisonRoundsSimilarPrices(t *testing.T) {
	rng := rand.New(rand.NewPCG(42, 0))

	// All products have very similar prices (< 5% difference between any pair)
	products := []models.Product{
		makeProduct("A", 100.0),
		makeProduct("B", 101.0),
		makeProduct("C", 102.0),
		makeProduct("D", 103.0),
	}

	_, err := GenerateComparisonRounds(products, 2, rng)
	assert.ErrorIs(t, err, ErrNotEnoughValidPairs)
}

func TestGenerateComparisonRoundsDeterministic(t *testing.T) {
	products := []models.Product{
		makeProduct("A", 100.0),
		makeProduct("B", 200.0),
		makeProduct("C", 50.0),
		makeProduct("D", 300.0),
		makeProduct("E", 150.0),
		makeProduct("F", 75.0),
	}

	// Same seed produces same output
	rng1 := rand.New(rand.NewPCG(99, 0))
	rng2 := rand.New(rand.NewPCG(99, 0))

	rounds1, err := GenerateComparisonRounds(products, 3, rng1)
	require.NoError(t, err)
	rounds2, err := GenerateComparisonRounds(products, 3, rng2)
	require.NoError(t, err)

	for i := range rounds1 {
		assert.Equal(t, rounds1[i].ProductA.ID, rounds2[i].ProductA.ID)
		assert.Equal(t, rounds1[i].ProductB.ID, rounds2[i].ProductB.ID)
		assert.Equal(t, rounds1[i].CorrectAnswer, rounds2[i].CorrectAnswer)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/game/ -v -run TestGenerateComparison -count=1`
Working directory: `backend/`
Expected: Compilation error — `GenerateComparisonRounds` undefined.

- [ ] **Step 4: Implement GenerateComparisonRounds in round_gen.go**

Update `backend/internal/game/round_gen.go` — replace the imports block and append the function after the existing type definitions. The full file should be:

```go
package game

import (
	"errors"
	"math/rand/v2"

	"github.com/google/uuid"
	"github.com/jzy/howmuchyousay/internal/models"
)

// MinPriceDiffPercent is the minimum price difference required between products in a comparison pair.
const MinPriceDiffPercent = 5.0

var (
	// ErrNotEnoughProducts is returned when the product pool is too small to generate the requested rounds.
	ErrNotEnoughProducts = errors.New("not enough products to generate rounds")
	// ErrNotEnoughValidPairs is returned when not enough product pairs meet the minimum price difference.
	ErrNotEnoughValidPairs = errors.New("not enough valid product pairs with sufficient price difference")
)

// RoundDef is the intermediate round definition before persisting to the database.
// It contains all data needed to create a Round row via RoundStore.Create().
type RoundDef struct {
	RoundNumber     int
	RoundType       models.RoundType
	ProductA        models.Product
	ProductB        *models.Product // nil for guess rounds
	CorrectAnswer   string
	DifficultyScore int
}

// GenerateComparisonRounds creates comparison round definitions from a product pool.
//
// Each round pairs two products with >= 5% price difference. No product is reused across rounds.
// The correct answer is "a" if product A is more expensive, "b" otherwise.
//
// Requirements:
//   - len(products) >= count * 2 (each round uses 2 unique products)
//   - Enough valid pairs with >= MinPriceDiffPercent price difference
func GenerateComparisonRounds(products []models.Product, count int, rng *rand.Rand) ([]RoundDef, error) {
	if len(products) < count*2 {
		return nil, ErrNotEnoughProducts
	}

	// Shuffle the products
	shuffled := make([]models.Product, len(products))
	copy(shuffled, products)
	rng.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	// Build valid pairs greedily from shuffled list
	var rounds []RoundDef
	used := make(map[uuid.UUID]bool)

	for i := 0; i < len(shuffled) && len(rounds) < count; i++ {
		if used[shuffled[i].ID] {
			continue
		}
		for j := i + 1; j < len(shuffled) && len(rounds) < count; j++ {
			if used[shuffled[j].ID] {
				continue
			}
			diff := PriceDiffPercent(shuffled[i].Price, shuffled[j].Price)
			if diff < MinPriceDiffPercent {
				continue
			}

			a := shuffled[i]
			b := shuffled[j]

			var correctAnswer string
			if a.Price > b.Price {
				correctAnswer = "a"
			} else {
				correctAnswer = "b"
			}

			rounds = append(rounds, RoundDef{
				RoundNumber:     len(rounds) + 1,
				RoundType:       models.RoundTypeComparison,
				ProductA:        a,
				ProductB:        &b,
				CorrectAnswer:   correctAnswer,
				DifficultyScore: ComparisonPoints(diff),
			})

			used[a.ID] = true
			used[b.ID] = true
			break
		}
	}

	if len(rounds) < count {
		return nil, ErrNotEnoughValidPairs
	}

	return rounds, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/game/ -v -run TestGenerateComparison -count=1`
Working directory: `backend/`
Expected: All tests PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/game/round_gen.go backend/internal/game/round_gen_test.go
git commit -m "feat(game): add comparison round generation with product pairing"
```

---

### Task 5: Round Generation — Guess Mode

**Files:**
- Modify: `backend/internal/game/round_gen.go`
- Modify: `backend/internal/game/round_gen_test.go`

Generates guess rounds from a product pool. Each round picks a single product. No reuse.

- [ ] **Step 1: Add guess round generation tests to round_gen_test.go**

Append to `backend/internal/game/round_gen_test.go`:

```go
func TestGenerateGuessRounds(t *testing.T) {
	rng := rand.New(rand.NewPCG(42, 0))

	products := []models.Product{
		makeProduct("Product A", 100.0),
		makeProduct("Product B", 200.0),
		makeProduct("Product C", 50.0),
		makeProduct("Product D", 300.0),
		makeProduct("Product E", 150.0),
		makeProduct("Product F", 75.0),
		makeProduct("Product G", 250.0),
		makeProduct("Product H", 120.0),
		makeProduct("Product I", 180.0),
		makeProduct("Product J", 90.0),
	}

	rounds, err := GenerateGuessRounds(products, 5, rng)
	require.NoError(t, err)
	assert.Len(t, rounds, 5)

	usedProductIDs := make(map[uuid.UUID]bool)
	for i, r := range rounds {
		// Round numbers are 1-indexed
		assert.Equal(t, i+1, r.RoundNumber)
		// Round type is guess
		assert.Equal(t, models.RoundTypeGuess, r.RoundType)
		// Product B is nil for guess rounds
		assert.Nil(t, r.ProductB)
		// Correct answer is the formatted price
		assert.Equal(t, FormatCorrectGuessAnswer(r.ProductA.Price), r.CorrectAnswer)
		// Difficulty score is 0 for guess rounds (points depend on player's answer)
		assert.Equal(t, 0, r.DifficultyScore)
		// No product reuse
		assert.False(t, usedProductIDs[r.ProductA.ID], "product reused in round %d", i+1)
		usedProductIDs[r.ProductA.ID] = true
	}
}

func TestGenerateGuessRoundsNotEnoughProducts(t *testing.T) {
	rng := rand.New(rand.NewPCG(42, 0))

	// Need 5 rounds = 5 products, only have 3
	products := []models.Product{
		makeProduct("A", 100.0),
		makeProduct("B", 200.0),
		makeProduct("C", 50.0),
	}

	_, err := GenerateGuessRounds(products, 5, rng)
	assert.ErrorIs(t, err, ErrNotEnoughProducts)
}

func TestGenerateGuessRoundsDeterministic(t *testing.T) {
	products := []models.Product{
		makeProduct("A", 100.0),
		makeProduct("B", 200.0),
		makeProduct("C", 50.0),
		makeProduct("D", 300.0),
		makeProduct("E", 150.0),
	}

	rng1 := rand.New(rand.NewPCG(99, 0))
	rng2 := rand.New(rand.NewPCG(99, 0))

	rounds1, err := GenerateGuessRounds(products, 3, rng1)
	require.NoError(t, err)
	rounds2, err := GenerateGuessRounds(products, 3, rng2)
	require.NoError(t, err)

	for i := range rounds1 {
		assert.Equal(t, rounds1[i].ProductA.ID, rounds2[i].ProductA.ID)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/game/ -v -run TestGenerateGuess -count=1`
Working directory: `backend/`
Expected: Compilation error — `GenerateGuessRounds` undefined.

- [ ] **Step 3: Implement GenerateGuessRounds in round_gen.go**

Add to `backend/internal/game/round_gen.go`:

```go
// GenerateGuessRounds creates guess round definitions from a product pool.
//
// Each round uses a single product. No product is reused across rounds.
// The correct answer is the product's price formatted as a string (e.g. "99.99").
// DifficultyScore is 0 for guess rounds — points depend entirely on the player's answer accuracy.
//
// Requirements:
//   - len(products) >= count
func GenerateGuessRounds(products []models.Product, count int, rng *rand.Rand) ([]RoundDef, error) {
	if len(products) < count {
		return nil, ErrNotEnoughProducts
	}

	// Shuffle and pick the first `count` products
	shuffled := make([]models.Product, len(products))
	copy(shuffled, products)
	rng.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	rounds := make([]RoundDef, count)
	for i := 0; i < count; i++ {
		rounds[i] = RoundDef{
			RoundNumber:     i + 1,
			RoundType:       models.RoundTypeGuess,
			ProductA:        shuffled[i],
			ProductB:        nil,
			CorrectAnswer:   FormatCorrectGuessAnswer(shuffled[i].Price),
			DifficultyScore: 0,
		}
	}

	return rounds, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/game/ -v -run TestGenerateGuess -count=1`
Working directory: `backend/`
Expected: All tests PASS.

- [ ] **Step 5: Run all round_gen tests together**

Run: `go test ./internal/game/ -v -run TestGenerate -count=1`
Working directory: `backend/`
Expected: All tests PASS (both comparison and guess).

- [ ] **Step 6: Commit**

```bash
git add backend/internal/game/round_gen.go backend/internal/game/round_gen_test.go
git commit -m "feat(game): add guess round generation"
```

---

### Task 6: Results Aggregation

**Files:**
- Create: `backend/internal/game/results.go`
- Create: `backend/internal/game/results_test.go`

Given all players, their answers, and round data for a session, produce a ranked leaderboard. Each player gets: rank, total points, correct answer count, total rounds played, and their best single-round score. Tied players share the same rank.

- [ ] **Step 1: Write results_test.go**

Create `backend/internal/game/results_test.go`:

```go
package game

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jzy/howmuchyousay/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalcResults(t *testing.T) {
	sessionID := uuid.New()

	player1 := models.Player{ID: uuid.New(), SessionID: sessionID, Nick: "Alice", IsHost: true}
	player2 := models.Player{ID: uuid.New(), SessionID: sessionID, Nick: "Bob", IsHost: false}

	round1 := models.Round{ID: uuid.New(), SessionID: sessionID, RoundNumber: 1}
	round2 := models.Round{ID: uuid.New(), SessionID: sessionID, RoundNumber: 2}
	round3 := models.Round{ID: uuid.New(), SessionID: sessionID, RoundNumber: 3}

	now := time.Now()

	answers := []models.Answer{
		// Round 1: Alice 3pts, Bob 0pts
		{ID: uuid.New(), RoundID: round1.ID, PlayerID: player1.ID, IsCorrect: true, PointsEarned: 3, AnsweredAt: now},
		{ID: uuid.New(), RoundID: round1.ID, PlayerID: player2.ID, IsCorrect: false, PointsEarned: 0, AnsweredAt: now},
		// Round 2: Alice 1pt, Bob 2pts
		{ID: uuid.New(), RoundID: round2.ID, PlayerID: player1.ID, IsCorrect: true, PointsEarned: 1, AnsweredAt: now},
		{ID: uuid.New(), RoundID: round2.ID, PlayerID: player2.ID, IsCorrect: true, PointsEarned: 2, AnsweredAt: now},
		// Round 3: Alice 0pts, Bob 5pts
		{ID: uuid.New(), RoundID: round3.ID, PlayerID: player1.ID, IsCorrect: false, PointsEarned: 0, AnsweredAt: now},
		{ID: uuid.New(), RoundID: round3.ID, PlayerID: player2.ID, IsCorrect: true, PointsEarned: 5, AnsweredAt: now},
	}

	rounds := []models.Round{round1, round2, round3}
	players := []models.Player{player1, player2}

	results := CalcResults(players, answers, rounds)
	require.Len(t, results, 2)

	// Bob wins: 7 points (0+2+5), Alice: 4 points (3+1+0)
	assert.Equal(t, 1, results[0].Rank)
	assert.Equal(t, "Bob", results[0].Nick)
	assert.Equal(t, player2.ID, results[0].PlayerID)
	assert.Equal(t, 7, results[0].TotalPoints)
	assert.Equal(t, 2, results[0].CorrectCount)
	assert.Equal(t, 3, results[0].TotalRounds)
	assert.Equal(t, 5, results[0].BestRoundScore)

	assert.Equal(t, 2, results[1].Rank)
	assert.Equal(t, "Alice", results[1].Nick)
	assert.Equal(t, player1.ID, results[1].PlayerID)
	assert.Equal(t, 4, results[1].TotalPoints)
	assert.Equal(t, 2, results[1].CorrectCount)
	assert.Equal(t, 3, results[1].TotalRounds)
	assert.Equal(t, 3, results[1].BestRoundScore)
}

func TestCalcResultsTiebreaker(t *testing.T) {
	sessionID := uuid.New()

	player1 := models.Player{ID: uuid.New(), SessionID: sessionID, Nick: "Alice"}
	player2 := models.Player{ID: uuid.New(), SessionID: sessionID, Nick: "Bob"}

	round1 := models.Round{ID: uuid.New(), SessionID: sessionID, RoundNumber: 1}

	now := time.Now()

	// Same points — tied players share the same rank
	answers := []models.Answer{
		{ID: uuid.New(), RoundID: round1.ID, PlayerID: player1.ID, IsCorrect: true, PointsEarned: 3, AnsweredAt: now},
		{ID: uuid.New(), RoundID: round1.ID, PlayerID: player2.ID, IsCorrect: true, PointsEarned: 3, AnsweredAt: now},
	}

	results := CalcResults([]models.Player{player1, player2}, answers, []models.Round{round1})
	require.Len(t, results, 2)

	// Both have 3 points — same rank
	assert.Equal(t, 1, results[0].Rank)
	assert.Equal(t, 1, results[1].Rank)
	assert.Equal(t, 3, results[0].TotalPoints)
	assert.Equal(t, 3, results[1].TotalPoints)
}

func TestCalcResultsNoAnswers(t *testing.T) {
	sessionID := uuid.New()
	player1 := models.Player{ID: uuid.New(), SessionID: sessionID, Nick: "Alice"}

	round1 := models.Round{ID: uuid.New(), SessionID: sessionID, RoundNumber: 1}

	results := CalcResults([]models.Player{player1}, nil, []models.Round{round1})
	require.Len(t, results, 1)

	assert.Equal(t, 1, results[0].Rank)
	assert.Equal(t, "Alice", results[0].Nick)
	assert.Equal(t, 0, results[0].TotalPoints)
	assert.Equal(t, 0, results[0].CorrectCount)
	assert.Equal(t, 1, results[0].TotalRounds)
	assert.Equal(t, 0, results[0].BestRoundScore)
}

func TestCalcResultsSinglePlayer(t *testing.T) {
	sessionID := uuid.New()
	player := models.Player{ID: uuid.New(), SessionID: sessionID, Nick: "Solo"}

	round1 := models.Round{ID: uuid.New(), SessionID: sessionID, RoundNumber: 1}
	round2 := models.Round{ID: uuid.New(), SessionID: sessionID, RoundNumber: 2}

	now := time.Now()
	answers := []models.Answer{
		{ID: uuid.New(), RoundID: round1.ID, PlayerID: player.ID, IsCorrect: true, PointsEarned: 5, AnsweredAt: now},
		{ID: uuid.New(), RoundID: round2.ID, PlayerID: player.ID, IsCorrect: true, PointsEarned: 2, AnsweredAt: now},
	}

	results := CalcResults([]models.Player{player}, answers, []models.Round{round1, round2})
	require.Len(t, results, 1)

	assert.Equal(t, 1, results[0].Rank)
	assert.Equal(t, "Solo", results[0].Nick)
	assert.Equal(t, 7, results[0].TotalPoints)
	assert.Equal(t, 2, results[0].CorrectCount)
	assert.Equal(t, 2, results[0].TotalRounds)
	assert.Equal(t, 5, results[0].BestRoundScore)
}

func TestCalcResultsThreePlayersWithTie(t *testing.T) {
	sessionID := uuid.New()

	player1 := models.Player{ID: uuid.New(), SessionID: sessionID, Nick: "Alice"}
	player2 := models.Player{ID: uuid.New(), SessionID: sessionID, Nick: "Bob"}
	player3 := models.Player{ID: uuid.New(), SessionID: sessionID, Nick: "Charlie"}

	round1 := models.Round{ID: uuid.New(), SessionID: sessionID, RoundNumber: 1}

	now := time.Now()

	// Alice: 5pts, Bob: 3pts, Charlie: 3pts — Bob and Charlie tied at rank 2
	answers := []models.Answer{
		{ID: uuid.New(), RoundID: round1.ID, PlayerID: player1.ID, IsCorrect: true, PointsEarned: 5, AnsweredAt: now},
		{ID: uuid.New(), RoundID: round1.ID, PlayerID: player2.ID, IsCorrect: true, PointsEarned: 3, AnsweredAt: now},
		{ID: uuid.New(), RoundID: round1.ID, PlayerID: player3.ID, IsCorrect: true, PointsEarned: 3, AnsweredAt: now},
	}

	results := CalcResults([]models.Player{player1, player2, player3}, answers, []models.Round{round1})
	require.Len(t, results, 3)

	assert.Equal(t, 1, results[0].Rank)
	assert.Equal(t, "Alice", results[0].Nick)
	assert.Equal(t, 2, results[1].Rank)
	assert.Equal(t, 2, results[2].Rank)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/game/ -v -run TestCalcResults -count=1`
Working directory: `backend/`
Expected: Compilation error — `CalcResults` and `PlayerScore` undefined.

- [ ] **Step 3: Implement results.go**

Create `backend/internal/game/results.go`:

```go
package game

import (
	"sort"

	"github.com/google/uuid"
	"github.com/jzy/howmuchyousay/internal/models"
)

// PlayerScore holds the aggregated results for a single player in a game session.
type PlayerScore struct {
	PlayerID       uuid.UUID `json:"player_id"`
	Nick           string    `json:"nick"`
	Rank           int       `json:"rank"`
	TotalPoints    int       `json:"total_points"`
	CorrectCount   int       `json:"correct_count"`
	TotalRounds    int       `json:"total_rounds"`
	BestRoundScore int       `json:"best_round_score"`
}

// CalcResults aggregates answers into player scores, sorted by total points descending.
//
// Parameters:
//   - players: all players in the session
//   - answers: all answers across all rounds in the session
//   - rounds: all rounds in the session (used only for total round count)
//
// Returns a slice of PlayerScore sorted by TotalPoints descending.
// Tied players share the same rank (e.g., 1st, 2nd, 2nd, 4th).
func CalcResults(players []models.Player, answers []models.Answer, rounds []models.Round) []PlayerScore {
	totalRounds := len(rounds)

	// Index answers by player ID
	type playerAcc struct {
		totalPoints    int
		correctCount   int
		bestRoundScore int
	}
	accByPlayer := make(map[uuid.UUID]*playerAcc)

	for i := range players {
		accByPlayer[players[i].ID] = &playerAcc{}
	}

	for _, a := range answers {
		acc, ok := accByPlayer[a.PlayerID]
		if !ok {
			continue
		}
		acc.totalPoints += a.PointsEarned
		if a.IsCorrect {
			acc.correctCount++
		}
		if a.PointsEarned > acc.bestRoundScore {
			acc.bestRoundScore = a.PointsEarned
		}
	}

	// Build results preserving player order
	results := make([]PlayerScore, len(players))
	for i, p := range players {
		acc := accByPlayer[p.ID]
		results[i] = PlayerScore{
			PlayerID:       p.ID,
			Nick:           p.Nick,
			TotalPoints:    acc.totalPoints,
			CorrectCount:   acc.correctCount,
			TotalRounds:    totalRounds,
			BestRoundScore: acc.bestRoundScore,
		}
	}

	// Stable sort by total points descending
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].TotalPoints > results[j].TotalPoints
	})

	// Assign ranks — tied players share the same rank
	for i := range results {
		if i == 0 || results[i].TotalPoints < results[i-1].TotalPoints {
			results[i].Rank = i + 1
		} else {
			results[i].Rank = results[i-1].Rank
		}
	}

	return results
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/game/ -v -run TestCalcResults -count=1`
Working directory: `backend/`
Expected: All tests PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/game/results.go backend/internal/game/results_test.go
git commit -m "feat(game): add results aggregation with player rankings"
```

---

### Task 7: Full Package Test Run & Cleanup

**Files:**
- Possibly modify: any file in `backend/internal/game/` for minor fixes

Final verification that all tests pass together and the package compiles cleanly.

- [ ] **Step 1: Run all game package tests**

Run: `go test ./internal/game/ -v -count=1`
Working directory: `backend/`
Expected: All tests PASS (scoring, comparison, guess, round generation comparison, round generation guess, results).

- [ ] **Step 2: Run go vet on the game package**

Run: `go vet ./internal/game/`
Working directory: `backend/`
Expected: No issues.

- [ ] **Step 3: Verify the full project still builds**

Run: `go build ./...`
Working directory: `backend/`
Expected: Build succeeds.

- [ ] **Step 4: Run all project tests (game + store)**

Run: `go test ./... -count=1`
Working directory: `backend/`
Expected: All tests PASS (game tests run without DB; store tests may skip if no test DB — that's fine).

- [ ] **Step 5: Commit any cleanup if needed**

Only if Steps 1-4 revealed issues that needed fixing:

```bash
git add -A
git commit -m "fix(game): cleanup from full test run"
```
