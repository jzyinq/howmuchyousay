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
		{"zero priceA", 0.0, 100.0, 0.0},
		{"zero priceB", 100.0, 0.0, 0.0},
		{"both zero", 0.0, 0.0, 0.0},
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
		{"zero actual", 50.0, 0.0, 0.0},
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
