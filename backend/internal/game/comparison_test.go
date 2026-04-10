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
