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
