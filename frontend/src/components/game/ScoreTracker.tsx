import { Badge } from "@/components/ui/badge"

interface ScoreTrackerProps {
  score: number
}

export default function ScoreTracker({ score }: ScoreTrackerProps) {
  return (
    <Badge variant="secondary" className="text-base px-4 py-1">
      Score: {score}
    </Badge>
  )
}
