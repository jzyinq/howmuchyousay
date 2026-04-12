import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"

interface RoundResultProps {
  isCorrect: boolean
  points: number
  isFinalRound: boolean
  onNext: () => void
}

export default function RoundResult({
  isCorrect,
  points,
  isFinalRound,
  onNext,
}: RoundResultProps) {
  return (
    <div className="flex flex-col items-center gap-4 mt-4">
      <Badge
        className={
          isCorrect
            ? "bg-green-400 text-black text-base px-4 py-2"
            : "bg-red-400 text-black text-base px-4 py-2"
        }
      >
        {isCorrect ? `Correct! +${points} points` : "Wrong! 0 points"}
      </Badge>
      <Button onClick={onNext}>
        {isFinalRound ? "See Results" : "Next Round"}
      </Button>
    </div>
  )
}
