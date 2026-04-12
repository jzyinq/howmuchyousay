import type { ApiError } from "@/types"

const BASE_URL = import.meta.env.VITE_API_URL ?? ""

export class ApiRequestError extends Error {
  status: number
  code: string
  details?: Record<string, unknown>

  constructor(err: ApiError) {
    super(err.message)
    this.name = "ApiRequestError"
    this.status = err.status
    this.code = err.code
    this.details = err.details
  }
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...options?.headers,
    },
  })

  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new ApiRequestError({
      status: res.status,
      code: body.code ?? "unknown",
      message: body.error ?? "Request failed",
      details: body,
    })
  }

  return res.json()
}

export function get<T>(path: string): Promise<T> {
  return request<T>(path)
}

export function post<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, {
    method: "POST",
    body: JSON.stringify(body),
  })
}
