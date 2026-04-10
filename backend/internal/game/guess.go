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
