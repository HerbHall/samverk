const BASE = '/api/v1'

// Read auth token injected by the Go server at serve time.
declare global {
  interface Window {
    __SAMVERK_TOKEN__?: string
    __SAMVERK_VERSION__?: string
    __SAMVERK_COMMIT__?: string
  }
}

function getAuthHeaders(): HeadersInit {
  const token = window.__SAMVERK_TOKEN__
  if (token) {
    return { Authorization: `Bearer ${token}` }
  }
  return {}
}

async function fetchJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...getAuthHeaders(),
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

export interface ProviderInfo {
  name: string
  type: string
  model: string
  healthy: boolean
  account_url?: string
}

export interface CapacityInfo {
  providers: ProviderInfo[]
  routing_chains: Record<string, string[]>
}

export interface MetricsResponse {
  pool: PoolMetrics | null
  dispatcher: DispatcherMetrics | null
  system: SystemMetrics | null
  pressure: PressureMetrics
  scaling_events: ScalingEvent[] | null
  scaling_config: ScalingConfig | null
  task_profiles: TaskProfile[] | null
  capacity: CapacityInfo | null
}

export interface HistoryEntry {
  timestamp: string
  pool: PoolMetrics | null
  dispatcher: DispatcherMetrics | null
  system: SystemMetrics | null
  pressure: PressureMetrics
}

export interface HistoryResponse {
  duration: string
  entries: HistoryEntry[]
}

export interface FailureEvent {
  issue_number: number
  issue_title: string
  agent_type: string
  provider: string
  error_category: string
  error_message: string
  timestamp: string
}

export interface FailureSummary {
  total_failures: number
  by_category: Record<string, number>
  by_provider: Record<string, number>
  recent: FailureEvent[]
}

export interface HostAlert {
  resource: string
  level: string
  message: string
  value: number
  threshold: number
}

export interface HostMetricsDTO {
  collected_at: string
  disk_total_bytes: number
  disk_used_bytes: number
  disk_percent: number
  ram_total_bytes: number
  ram_used_bytes: number
  ram_avail_bytes: number
  ram_percent: number
  swap_total_bytes: number
  swap_used_bytes: number
  swap_percent: number
  load_avg_1: number
  load_avg_5: number
  load_avg_15: number
  num_cpu: number
  alerts?: HostAlert[]
}

export interface HostMetricsResponse {
  current: HostMetricsDTO
  history: HostMetricsDTO[]
}

export interface LogEntry {
  id: number
  ts: string
  level: string
  msg: string
  component?: string
  session_id?: string
  issue_number?: number
  fields?: string
}

export interface LogSummary {
  scope: { type: string; id: string }
  text: string
  total_entries: number
  error_count: number
  warn_count: number
  components: string[]
  ai_generated: boolean
  generated_at: string
}

// Synapset types (matching actual upstream API responses)
export interface SynapsetStatus {
  version: string
  uptime_seconds: number
  memory_count: number
  pool_count: number
  db_size_bytes: number
  provider: string
}

export interface SynapsetToolSummaryEntry {
  ToolName: string
  CallCount: number
  AvgDurationMs: number
  MaxDurationMs: number
  ErrorCount: number
  AvgSimilarity: number
}

export interface SynapsetToolHistoryEntry {
  ID: number
  ToolName: string
  PoolName: string
  DurationMs: number
  Status: string
  ErrorMsg: string
  ResultCount: number
  SimilarityAvg: number
  CreatedAt: string
}

export interface SynapsetSearchStats {
  total_searches: number
  avg_similarity: number
  avg_result_count: number
  similarity_distribution: Record<string, number>
}

export interface SynapsetMemoryTrendEntry {
  date: string
  pool_name: string
  count: number
  cumulative: number
}

export interface SynapsetPoolStatsEntry {
  name: string
  description: string
  model_name: string
  dimensions: number
  created_at: string
  updated_at: string
}

export interface SynapsetMetricsHistoryEntry {
  ID: number
  Goroutines: number
  HeapAllocMB: number
  HeapSysMB: number
  GCCycles: number
  DBSizeBytes: number
  MemoryCount: number
  PoolCount: number
  UptimeSeconds: number
  CreatedAt: string
}

export interface SynapsetLogEntry {
  ID: number
  Timestamp: string
  Level: string
  Message: string
  Component: string
  Fields: string
  CreatedAt: string
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

  getMetricsHistory: (duration?: string) =>
    fetchJSON<HistoryResponse>(`/metrics/history${duration ? '?duration=' + duration : ''}`),

  getFailures: (hours?: number) =>
    fetchJSON<FailureSummary>(`/failures${hours ? '?hours=' + hours : ''}`),

  getHostMetrics: () =>
    fetchJSON<HostMetricsResponse>('/metrics/host'),

  getLogs: (params?: {
    level?: string
    component?: string
    session_id?: string
    issue?: number
    since?: string
    until?: string
    limit?: number
    offset?: number
    q?: string
  }) => {
    const searchParams = new URLSearchParams()
    if (params) {
      Object.entries(params).forEach(([k, v]) => {
        if (v !== undefined && v !== '') searchParams.set(k, String(v))
      })
    }
    const qs = searchParams.toString()
    return fetchJSON<LogEntry[]>(`/logs${qs ? '?' + qs : ''}`)
  },

  getLogSummary: (params: { scope: string; id?: string; since?: string; until?: string }) => {
    const searchParams = new URLSearchParams()
    Object.entries(params).forEach(([k, v]) => {
      if (v !== undefined && v !== '') searchParams.set(k, String(v))
    })
    return fetchJSON<LogSummary>(`/logs/summary?${searchParams.toString()}`)
  },

  // Synapset proxy endpoints
  synapsetStatus: () =>
    fetchJSON<SynapsetStatus>('/synapset/status'),

  synapsetToolSummary: (period: string) =>
    fetchJSON<SynapsetToolSummaryEntry[]>(`/synapset/tools/summary?period=${period}`),

  synapsetToolHistory: (period: string) =>
    fetchJSON<SynapsetToolHistoryEntry[]>(`/synapset/tools/history?period=${period}`),

  synapsetSearchStats: (period: string) =>
    fetchJSON<SynapsetSearchStats>(`/synapset/search/stats?period=${period}`),

  synapsetMemoryTrend: (period: string) =>
    fetchJSON<SynapsetMemoryTrendEntry[]>(`/synapset/memories/trend?period=${period}`),

  synapsetPoolStats: () =>
    fetchJSON<SynapsetPoolStatsEntry[]>('/synapset/pools/stats'),

  synapsetMetricsHistory: (period: string) =>
    fetchJSON<SynapsetMetricsHistoryEntry[]>(`/synapset/metrics/history?period=${period}`),

  synapsetLogs: (limit?: number) =>
    fetchJSON<SynapsetLogEntry[]>(`/synapset/logs${limit ? '?limit=' + limit : ''}`),
}
