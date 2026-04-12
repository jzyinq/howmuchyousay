import { useState, useEffect } from "react"
import { useParams, useNavigate, Link } from "react-router"
import { useSession, useRound, useSubmitAnswer } from "@/api/gameApi"
import { ApiRequestError } from "@/api/client"
import { Button } from "@/components/ui/button"
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
  const [submitError, setSubmitError] = useState<string | null>(null)

  useEffect(() => {
    if (session && localRound === 0) {
      setLocalRound(session.current_round)
    }
  }, [session, localRound])

  // Redirect finished sessions to results
  useEffect(() => {
    if (session?.status === "finished") {
      navigate(`/game/${sessionId}/results`, { replace: true })
    }
  }, [session?.status, sessionId, navigate])

  const {
    data: round,
    isLoading: roundLoading,
    error: roundError,
    refetch: refetchRound,
  } = useRound(sessionId!, localRound)
  const submitAnswer = useSubmitAnswer(sessionId!, localRound)

  function handleSubmitAnswer(answer: "a" | "b") {
    setSubmitError(null)
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
      onError: (err) => {
        if (err instanceof ApiRequestError && err.code === "already_answered") {
          // Treat as success — extract data from error details
          const details = err.details
          if (details) {
            setCurrentResult({
              is_correct: details.is_correct as boolean,
              points: details.points as number,
              correct_answer: details.correct_answer as string,
              price_a: details.price_a as number,
              price_b: details.price_b as number | undefined,
            })
            setRoundPhase("result")
          }
        } else {
          setSubmitError("Failed to submit answer. Please try again.")
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
    setSubmitError(null)
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

      {roundError ? (
        <div className="flex flex-col items-center gap-4 mt-8">
          <p className="text-foreground font-heading">Failed to load round</p>
          <Button onClick={() => refetchRound()}>Retry</Button>
        </div>
      ) : roundLoading ? (
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
          {submitError && (
            <p className="text-sm font-base text-red-600 border-2 border-red-600 rounded-base p-2 bg-red-50">
              {submitError}
            </p>
          )}
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
