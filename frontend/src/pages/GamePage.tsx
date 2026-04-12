import { useState, useEffect } from "react"
import { useParams, useNavigate, Link } from "react-router"
import { useSession, useRound, useSubmitAnswer } from "@/api/gameApi"
import ComparisonRound from "@/components/game/ComparisonRound"
import RoundResult from "@/components/game/RoundResult"
import RoundCounter from "@/components/game/RoundCounter"
import ScoreTracker from "@/components/game/ScoreTracker"
import type { AnswerResponse, RoundHistoryEntry } from "@/types"

export default function GamePage() {
  const { sessionId } = useParams<{ sessionId: string }>()
  const navigate = useNavigate()

  const { data: session, isLoading: sessionLoading, error: sessionError } = useSession(sessionId!)
  const [localRound, setLocalRound] = useState(0)
  const [totalScore, setTotalScore] = useState(0)
  const [roundPhase, setRoundPhase] = useState<"answering" | "result">("answering")
  const [currentResult, setCurrentResult] = useState<AnswerResponse | null>(null)
  const [roundHistory, setRoundHistory] = useState<RoundHistoryEntry[]>([])

  useEffect(() => {
    if (session && localRound === 0) {
      setLocalRound(session.current_round)
    }
  }, [session, localRound])

  const { data: round, isLoading: roundLoading } = useRound(sessionId!, localRound)
  const submitAnswer = useSubmitAnswer(sessionId!, localRound)

  function handleSubmitAnswer(answer: "a" | "b") {
    submitAnswer.mutate(answer, {
      onSuccess: (result) => {
        setCurrentResult(result)
        setTotalScore((prev) => prev + result.points)
        setRoundPhase("result")

        if (round && round.product_a && round.product_b) {
          setRoundHistory((prev) => [
            ...prev,
            {
              round_number: localRound,
              product_a: round.product_a,
              product_b: round.product_b!,
              selected_answer: answer,
              is_correct: result.is_correct,
              points: result.points,
              correct_answer: result.correct_answer,
            },
          ])
        }
      },
    })
  }

  function handleNextRound() {
    if (session && localRound >= session.rounds_total) {
      navigate(`/game/${sessionId}/results`, {
        state: { roundHistory, totalScore: totalScore },
      })
      return
    }
    setLocalRound((prev) => prev + 1)
    setRoundPhase("answering")
    setCurrentResult(null)
  }

  if (!sessionId) {
    navigate("/")
    return null
  }

  if (sessionLoading) {
    return (
      <div className="min-h-screen bg-background flex items-center justify-center">
        <p className="text-foreground font-heading">Loading game...</p>
      </div>
    )
  }

  if (sessionError || !session) {
    return (
      <div className="min-h-screen bg-background flex flex-col items-center justify-center gap-4">
        <p className="text-foreground font-heading">Game not found</p>
        <Link to="/" className="text-main underline font-heading">
          Back to Home
        </Link>
      </div>
    )
  }

  if (session.status === "finished") {
    navigate(`/game/${sessionId}/results`)
    return null
  }

  if (session.status === "failed") {
    return (
      <div className="min-h-screen bg-background flex flex-col items-center justify-center gap-4">
        <p className="text-foreground font-heading">Game failed</p>
        <p className="text-foreground/70">{session.error_message}</p>
        <Link to="/play" className="text-main underline font-heading">
          Try Again
        </Link>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-background flex flex-col items-center p-4 gap-6">
      <Link to="/" className="text-2xl font-heading text-foreground hover:underline">
        HowMuchYouSay
      </Link>
      <div className="flex items-center gap-6 w-full max-w-md justify-between">
        <RoundCounter
          current={localRound}
          total={session.rounds_total}
        />
        <ScoreTracker score={totalScore} />
      </div>

      {roundLoading ? (
        <p className="text-foreground font-heading mt-8">Loading round...</p>
      ) : round && round.product_a && round.product_b ? (
        <>
          <ComparisonRound
            key={localRound}
            productA={round.product_a}
            productB={round.product_b}
            onSubmit={handleSubmitAnswer}
            isSubmitting={submitAnswer.isPending}
            result={roundPhase === "result" ? currentResult : null}
          />
          {roundPhase === "result" && currentResult && (
            <RoundResult
              isCorrect={currentResult.is_correct}
              points={currentResult.points}
              isFinalRound={localRound >= session.rounds_total}
              onNext={handleNextRound}
            />
          )}
        </>
      ) : (
        <p className="text-foreground font-heading mt-8">
          Waiting for round data...
        </p>
      )}
    </div>
  )
}
