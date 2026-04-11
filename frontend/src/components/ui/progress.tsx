import * as React from "react"
import { cn } from "@/lib/utils"

interface ProgressProps extends React.ComponentProps<"div"> {
  value?: number
  max?: number
}

function Progress({ className, value = 0, max = 100, ...props }: ProgressProps) {
  const percentage = Math.min(Math.max((value / max) * 100, 0), 100)

  return (
    <div
      data-slot="progress"
      role="progressbar"
      aria-valuenow={value}
      aria-valuemin={0}
      aria-valuemax={max}
      className={cn(
        "relative h-4 w-full overflow-hidden rounded-base border-2 border-border bg-secondary-background",
        className,
      )}
      {...props}
    >
      <div
        className="h-full bg-main transition-all"
        style={{ width: `${percentage}%` }}
      />
    </div>
  )
}

export { Progress }
