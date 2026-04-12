import { Progress } from "@/components/ui/progress"

interface RoundCounterProps {
  current: number
  total: number
}

export default function RoundCounter({ current, total }: RoundCounterProps) {
  return (
    <div className="w-full max-w-md flex flex-col gap-2">
      <span className="text-sm font-heading text-foreground">
        Round {current} / {total}
      </span>
      <Progress value={current} max={total} />
    </div>
  )
}
