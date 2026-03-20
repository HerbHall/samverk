const BASE = '/api/v1'

// Version/commit info injected by the Go server at serve time.
declare global {
  interface Window {
    __SAMVERK_VERSION__?: string
    __SAMVERK_COMMIT__?: string
  }
}

async function fetchJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    credentials: 'same-origin',
    headers: {
      'Content-Type': 'application/json',
      ...init?.headers,
    },
  })
  if (!res.ok) {
    // If session expired, redirect to login.
    if (res.status === 401 || res.status === 303) {
      window.location.href = '/login'
      throw new Error('Session expired')
    }
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

export interface ActiveWorker {
  session_id: string
  issue_number: number
  agent_type: string
  provider: string
  model: string
  started_at: string
  elapsed_s: number
}

export interface CostSummary {
  tokens_used: number
  estimated_cost_usd: number
  budget_remaining_usd: number
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
  cpu_percent: number
  cpu_source?: string
  in_lxc: boolean
  alerts?: HostAlert[]
}

export interface HostRecommendationAction {
  description: string
  ssh_target: string
  command: string
  current_value: string
  recommended_value: string
  cost_tier: string   // "free" | "hardware"
  cost_note: string
}

export interface HostRecommendationStats {
  avg: number
  p95: number
  peak: number
  time_above_warn: number
  time_above_crit: number
  days_to_warn: number
  days_to_crit: number
  sample_count: number
}

export interface HostRecommendation {
  resource: string
  level: 'info' | 'warn' | 'critical'
  title: string
  detail: string
  action_type: string    // "proxmox_config" | "hardware_upgrade" | "none"
  action?: HostRecommendationAction
  stats: HostRecommendationStats
}

export interface HostMetricsResponse {
  current: HostMetricsDTO
  history: HostMetricsDTO[]
  recommendations?: HostRecommendation[]
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

// Agent (enhanced provider) types
export interface AgentStats {
  total_sessions: number
  completed: number
  failed: number
  active: number
  success_rate: number
  avg_duration_secs: number
}

export interface AgentSession {
  id: string
  issue_number: number
  status: string
  started_at: string
  duration_secs?: number
}

export interface ThroughputPoint {
  hour: string
  count: number
}

export interface AgentInfo {
  name: string
  type: string
  model: string
  healthy: boolean
  account_url?: string
  routing_chains: string[]
  stats: AgentStats
  recent_sessions: AgentSession[]
  throughput: ThroughputPoint[]
}

export interface AgentsResponse {
  agents: AgentInfo[]
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

export interface ProviderHealthEntry {
  name: string
  healthy: boolean
  last_checked?: string
  last_healthy?: string
  last_success_at?: string
  error?: string
  model_loaded?: boolean
  vram_free_bytes?: number
  vram_total_bytes?: number
}

export interface ProviderHealthResponse {
  providers: ProviderHealthEntry[]
  count: number
  routing_chains?: Record<string, string[]>
}

export interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
}

export interface ChatRequest {
  messages: ChatMessage[]
  system: string
}

export interface ChatResponse {
  content: string
  model: string
  input_tokens: number
  output_tokens: number
}

export const api = {
  listIssues: async (params?: { state?: string; page?: number }) => {
    const data = await fetchJSON<{ issues: Issue[]; total: number }>(`/issues${params ? '?' + new URLSearchParams(params as Record<string, string>).toString() : ''}`)
    return data.issues
  },

  searchIssues: async (q: string, state?: string) => {
    const params = new URLSearchParams({ q })
    if (state) params.set('state', state)
    const data = await fetchJSON<{ issues: Issue[]; total: number }>(`/issues/search?${params.toString()}`)
    return data.issues
  },

  getIssue: (number: number) =>
    fetchJSON<Issue>(`/issues/${number}`),

  listSessions: () =>
    fetchJSON<Session[]>('/sessions'),

  getActiveWorkers: () =>
    fetchJSON<ActiveWorker[]>('/workers/active'),

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

  getAgents: () =>
    fetchJSON<AgentsResponse>('/agents'),

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

  getProviderHealth: () =>
    fetchJSON<ProviderHealthResponse>('/providers/health'),

  sendChat: (req: ChatRequest) =>
    fetchJSON<ChatResponse>('/chat', {
      method: 'POST',
      body: JSON.stringify(req),
    }),

  applyCapacityRecommendation: (resource: string) =>
    fetchJSON<{ issue_number: number; issue_url: string }>('/capacity/apply', {
      method: 'POST',
      body: JSON.stringify({ resource }),
    }),

  bulkIssues: (action: 'close' | 'label' | 'assign', issueNumbers: number[], value?: string) =>
    fetchJSON<{ succeeded: number[]; failed: { number: number; error: string }[] }>('/issues/bulk', {
      method: 'POST',
      body: JSON.stringify({ action, issue_numbers: issueNumbers, value }),
    }),
}
