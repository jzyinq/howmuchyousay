import { useMutation, useQuery } from "@tanstack/react-query"
import { get, post } from "./client"
import type {
  CreateGameRequest,
  CreateGameResponse,
  SessionResponse,
  RoundResponse,
  AnswerResponse,
  ResultsResponse,
} from "@/types"

export function useCreateGame() {
  return useMutation({
    mutationFn: (data: CreateGameRequest) =>
      post<CreateGameResponse>("/api/game", data),
  })
}

export function useSession(sessionId: string) {
  return useQuery({
    queryKey: ["session", sessionId],
    queryFn: () => get<SessionResponse>(`/api/game/${sessionId}`),
    staleTime: Infinity,
  })
}

export function useRound(sessionId: string, roundNumber: number) {
  return useQuery({
    queryKey: ["round", sessionId, roundNumber],
    queryFn: () =>
      get<RoundResponse>(`/api/game/${sessionId}/round/${roundNumber}`),
    staleTime: Infinity,
    enabled: roundNumber > 0,
  })
}

export function useSubmitAnswer(sessionId: string, roundNumber: number) {
  return useMutation({
    mutationFn: (answer: string) =>
      post<AnswerResponse>(
        `/api/game/${sessionId}/round/${roundNumber}/answer`,
        { answer },
      ),
  })
}

export function useResults(sessionId: string) {
  return useQuery({
    queryKey: ["results", sessionId],
    queryFn: () => get<ResultsResponse>(`/api/game/${sessionId}/results`),
    staleTime: Infinity,
  })
}
