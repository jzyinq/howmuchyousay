export type GameMode = "comparison" | "guess"
export type GameStatus = "crawling" | "ready" | "in_progress" | "finished" | "failed"

export interface CreateGameRequest {
  nick: string
  shop_url: string
  game_mode: GameMode
  skip_crawl: boolean
}

export interface CreateGameResponse {
  session_id: string
}

export interface SessionResponse {
  id: string
  status: GameStatus
  game_mode: GameMode
  rounds_total: number
  current_round: number
  error_message?: string
}

export interface ProductDTO {
  id: string
  name: string
  image_url: string
}

export interface RoundResponse {
  number: number
  type: string
  product_a: ProductDTO
  product_b?: ProductDTO
}

export interface AnswerResponse {
  is_correct: boolean
  points: number
  correct_answer: string
  price_a: number
  price_b?: number
}

export interface PlayerScore {
  player_id: string
  nick: string
  rank: number
  total_points: number
  correct_count: number
  total_rounds: number
  best_round_score: number
}

export interface ResultsResponse {
  session_id: string
  rankings: PlayerScore[]
}

export interface RoundHistoryEntry {
  round_number: number
  product_a: ProductDTO
  product_b: ProductDTO
  selected_answer: "a" | "b"
  is_correct: boolean
  points: number
  correct_answer: string
}

export interface ApiError {
  status: number
  code: string
  message: string
  details?: Record<string, unknown>
}
