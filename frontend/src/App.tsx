import { Routes, Route } from "react-router"
import HomePage from "@/pages/HomePage"
import SetupPage from "@/pages/SetupPage"
import GamePage from "@/pages/GamePage"
import ResultsPage from "@/pages/ResultsPage"

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<HomePage />} />
      <Route path="/play" element={<SetupPage />} />
      <Route path="/game/:sessionId" element={<GamePage />} />
      <Route path="/game/:sessionId/results" element={<ResultsPage />} />
    </Routes>
  )
}
