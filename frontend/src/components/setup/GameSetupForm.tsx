import { useState } from "react"
import { useNavigate } from "react-router"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Card, CardHeader, CardTitle, CardContent, CardFooter } from "@/components/ui/card"
import { useCreateGame } from "@/api/gameApi"
import { ApiRequestError } from "@/api/client"

export default function GameSetupForm() {
  const navigate = useNavigate()
  const createGame = useCreateGame()

  const [nick, setNick] = useState("")
  const [shopUrl, setShopUrl] = useState("")
  const [error, setError] = useState<string | null>(null)

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)

    if (!nick.trim() || nick.length > 32) {
      setError("Nick is required (1-32 characters)")
      return
    }

    const trimmed = shopUrl.trim()
    if (!trimmed || !trimmed.includes(".")) {
      setError("Please enter a valid shop domain (e.g. allegro.pl)")
      return
    }

    createGame.mutate(
      {
        nick: nick.trim(),
        shop_url: shopUrl.trim(),
        game_mode: "comparison",
        skip_crawl: true,
      },
      {
        onSuccess: (data) => {
          navigate(`/game/${data.session_id}`)
        },
        onError: (err) => {
          if (err instanceof ApiRequestError) {
            if (err.code === "not_enough_products") {
              setError(
                "Not enough products in this shop's database. Try a different shop URL.",
              )
            } else {
              setError(err.message)
            }
          } else {
            setError("Something went wrong. Please try again.")
          }
        },
      },
    )
  }

  return (
    <Card className="w-full max-w-md">
      <form onSubmit={handleSubmit}>
        <CardHeader>
          <CardTitle>New Game</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label htmlFor="nick">Your Nick</Label>
            <Input
              id="nick"
              placeholder="Enter your nickname"
              value={nick}
              onChange={(e) => setNick(e.target.value)}
              maxLength={32}
              required
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="shop-url">Shop URL</Label>
            <Input
              id="shop-url"
              placeholder="allegro.pl"
              type="text"
              value={shopUrl}
              onChange={(e) => setShopUrl(e.target.value)}
              required
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label>Game Mode</Label>
            <div className="flex h-10 w-full items-center rounded-base border-2 border-border bg-secondary-background px-3 text-sm">
              Comparison (Which is more expensive?)
            </div>
          </div>
          {error && (
            <p className="text-sm font-base text-red-600 border-2 border-red-600 rounded-base p-2 bg-red-50">
              {error}
            </p>
          )}
        </CardContent>
        <CardFooter>
          <Button
            type="submit"
            className="w-full"
            disabled={createGame.isPending}
          >
            {createGame.isPending ? "Starting..." : "Start Game"}
          </Button>
        </CardFooter>
      </form>
    </Card>
  )
}
