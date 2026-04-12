import { Link } from "react-router"
import GameSetupForm from "@/components/setup/GameSetupForm"

export default function SetupPage() {
  return (
    <div className="min-h-screen bg-background flex flex-col items-center justify-center gap-6 p-4">
      <Link to="/" className="text-3xl font-heading text-foreground hover:underline">
        HowMuchYouSay
      </Link>
      <GameSetupForm />
    </div>
  )
}
