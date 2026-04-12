import { Routes, Route, Link } from "react-router"
import HomePage from "@/pages/HomePage"
import SetupPage from "@/pages/SetupPage"
import GamePage from "@/pages/GamePage"
import ResultsPage from "@/pages/ResultsPage"
import { Button } from "@/components/ui/button"

function NotFound() {
  return (
    <div className="min-h-screen bg-background flex flex-col items-center justify-center gap-4">
      <h1 className="text-4xl font-heading">404</h1>
      <p className="text-foreground/70">Page not found</p>
      <Button asChild>
        <Link to="/">Back to Home</Link>
      </Button>
    </div>
  )
}

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<HomePage />} />
      <Route path="/play" element={<SetupPage />} />
      <Route path="/game/:sessionId" element={<GamePage />} />
      <Route path="/game/:sessionId/results" element={<ResultsPage />} />
      <Route path="*" element={<NotFound />} />
    </Routes>
  )
}
