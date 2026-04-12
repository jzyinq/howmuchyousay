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

  const hasState = !!state && state.roundHistory.length > 0
  const { data: results, isLoading } = useResults(sessionId!, !hasState)

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
