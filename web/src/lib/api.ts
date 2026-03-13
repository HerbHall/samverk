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
  forge_connected: boolean
  database_connected: boolean
  tool_count: number
}

export interface PoolMetrics {
  collected_at: string
  total_workers: number
  active_workers: number
  idle_workers: number
  queue_depth: number
  tasks_completed: number
  tasks_failed: number
  avg_task_duration_ms: number
  p95_task_duration_ms: number
}

export interface DispatcherMetrics {
  collected_at: string
  claimed_count: number
  total_routed: number
  total_requeued: number
  total_events_processed: number
  avg_poll_latency_ms: number
  last_poll_at: string
}

export interface SystemMetrics {
  collected_at: string
  goroutines: number
  heap_alloc_bytes: number
  sys_bytes_total: number
  gc_cycles: number
  next_gc_bytes: number
}

export interface PressureMetrics {
  level: string
  reasons: string[] | null
}

export interface ScalingEvent {
  timestamp: string
  action: string
  from_workers: number
  to_workers: number
  reason: string
  confidence: number
}

export interface ScalingConfig {
  enabled: boolean
  min_workers: number
  max_workers: number
  current_target: number
}

export interface TaskProfile {
  agent_type: string
  provider: string
  avg_duration_ms: number
  p50_duration_ms: number
  p90_duration_ms: number
  sample_count: number
  avg_tokens: number
  updated_at: string
}

export interface MetricsResponse {
  pool: PoolMetrics | null
  dispatcher: DispatcherMetrics | null
  system: SystemMetrics | null
  pressure: PressureMetrics
  scaling_events: ScalingEvent[] | null
  scaling_config: ScalingConfig | null
  task_profiles: TaskProfile[] | null
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

  getMetrics: () =>
    fetchJSON<MetricsResponse>('/metrics'),
}
