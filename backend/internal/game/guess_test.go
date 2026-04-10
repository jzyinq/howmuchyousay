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
