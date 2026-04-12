import { Link } from "react-router"
import { Button } from "@/components/ui/button"

export default function HomePage() {
  return (
    <div className="min-h-screen bg-background flex flex-col items-center justify-center gap-8">
      <div className="text-center">
        <h1 className="text-6xl font-heading text-foreground">
          HowMuchYouSay
        </h1>
        <p className="mt-4 text-lg text-foreground/70">
          Guess which product costs more!
        </p>
      </div>
      <Button size="lg" asChild>
        <Link to="/play">Play</Link>
      </Button>
    </div>
  )
}
