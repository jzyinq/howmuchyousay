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
