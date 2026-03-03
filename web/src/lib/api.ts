const BASE = '/api/v1'

async function fetchJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...init?.headers,
    },
  })
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`API ${res.status}: ${body}`)
  }
  return res.json()
}

export interface Issue {
  number: number
  title: string
  body: string
  state: string
  labels: string[]
  assignees: string[]
  created_at: string
  updated_at: string
  closed_at: string | null
}

export interface Session {
  id: string
  issue_number: number
  agent_type: string
  provider: string
  model: string
  status: string
  started_at: string
  finished_at: string | null
}

export interface CostSummary {
  total_cost_usd: number
  total_input_tokens: number
  total_output_tokens: number
  record_count: number
}

export interface SystemStatus {
  healthy: boolean
  github_connected: boolean
  database_connected: boolean
  tool_count: number
}

export const api = {
  listIssues: (params?: { state?: string; page?: number }) =>
    fetchJSON<Issue[]>(`/issues${params ? '?' + new URLSearchParams(params as Record<string, string>).toString() : ''}`),

  getIssue: (number: number) =>
    fetchJSON<Issue>(`/issues/${number}`),

  listSessions: () =>
    fetchJSON<Session[]>('/sessions'),

  getCosts: () =>
    fetchJSON<CostSummary>('/costs'),

  getStatus: () =>
    fetchJSON<SystemStatus>('/status'),
}
