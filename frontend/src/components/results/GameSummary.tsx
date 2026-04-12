import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import type { RoundHistoryEntry } from "@/types"

interface GameSummaryProps {
  totalScore: number
  roundHistory: RoundHistoryEntry[]
}

export default function GameSummary({ totalScore, roundHistory }: GameSummaryProps) {
  const correctCount = roundHistory.filter((r) => r.is_correct).length

  return (
    <Card className="w-full max-w-2xl">
      <CardHeader className="text-center">
        <CardTitle className="text-3xl">Game Over!</CardTitle>
        <div className="flex justify-center gap-4 mt-2">
          <Badge className="text-xl px-6 py-2">
            Score: {totalScore}
          </Badge>
          <Badge variant="secondary" className="text-xl px-6 py-2">
            {correctCount} / {roundHistory.length} correct
          </Badge>
        </div>
      </CardHeader>
      <CardContent>
        <div className="flex flex-col gap-3">
          {roundHistory.map((entry) => (
            <div
              key={entry.round_number}
              className="flex items-center gap-3 p-3 rounded-base border-2 border-border bg-background"
            >
              <span className="text-sm font-heading w-20 shrink-0">
                Round {entry.round_number}
              </span>
              <span className="text-sm flex-1 truncate">
                {entry.product_a.name} vs {entry.product_b.name}
              </span>
              <Badge
                className={
                  entry.is_correct
                    ? "bg-green-400 text-black"
                    : "bg-red-400 text-black"
                }
              >
                {entry.is_correct ? `+${entry.points}` : "0"}
              </Badge>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  )
}
