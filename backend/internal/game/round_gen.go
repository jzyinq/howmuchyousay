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
