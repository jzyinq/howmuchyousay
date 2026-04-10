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
