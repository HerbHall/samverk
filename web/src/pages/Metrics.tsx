import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import type {
  PoolMetrics, DispatcherMetrics, SystemMetrics, PressureMetrics,
  ScalingEvent, ScalingConfig, CapacityInfo, Issue, Session, HistoryEntry,
} from '../lib/api'

function formatBytes(bytes: number): string {
  if (bytes >= 1_073_741_824) return `${(bytes / 1_073_741_824).toFixed(1)} GB`
  if (bytes >= 1_048_576) return `${(bytes / 1_048_576).toFixed(1)} MB`
  if (bytes >= 1_024) return `${(bytes / 1_024).toFixed(1)} KB`
  return `${bytes} B`
}

function formatMs(ms: number): string {
  if (ms === 0) return '—'
  if (ms >= 1_000) return `${(ms / 1_000).toFixed(2)}s`
  return `${ms.toFixed(1)}ms`
}

function formatRelative(dateStr: string): string {
  const diffMs = Date.now() - new Date(dateStr).getTime()
  const m = Math.floor(diffMs / 60_000)
  const h = Math.floor(diffMs / 3_600_000)
  const d = Math.floor(diffMs / 86_400_000)
  if (m < 1) return 'just now'
  if (m < 60) return `${m}m ago`
  if (h < 24) return `${h}h ago`
  if (d < 30) return `${d}d ago`
  return new Date(dateStr).toLocaleDateString()
}

function MetricRow({ label, value, tooltip }: { label: string; value: string; tooltip?: string }) {
  return (
    <div className="flex items-center justify-between py-2 border-b last:border-b-0">
      {tooltip ? (
        <span className="group relative text-sm text-gray-600 cursor-help underline decoration-dotted decoration-gray-400 underline-offset-2">
          {label}
          <span className="pointer-events-none absolute left-0 bottom-full mb-1 z-10 hidden w-64 rounded bg-gray-900 px-3 py-2 text-xs leading-relaxed text-gray-100 shadow-lg group-hover:block">
            {tooltip}
          </span>
        </span>
      ) : (
        <span className="text-sm text-gray-600">{label}</span>
      )}
      <span className="text-sm font-medium text-gray-900 font-mono">{value}</span>
    </div>
  )
}

function SectionCard({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="rounded-lg border bg-white">
      <div className="px-4 py-3 border-b bg-gray-50">
        <h3 className="text-sm font-semibold text-gray-700">{title}</h3>
      </div>
      <div className="px-4 py-2">{children}</div>
    </div>
  )
}

function NullSection({ title }: { title: string }) {
  return (
    <div className="rounded-lg border bg-white">
      <div className="px-4 py-3 border-b bg-gray-50">
        <h3 className="text-sm font-semibold text-gray-400">{title}</h3>
      </div>
      <div className="px-4 py-6 text-center text-sm text-gray-400">Not running</div>
    </div>
  )
}

function StatCard({ label, value, sub, color }: { label: string; value: string; sub?: string; color?: string }) {
  return (
    <div className="rounded-lg border bg-white px-4 py-3">
      <p className="text-xs font-medium text-gray-500 uppercase tracking-wide">{label}</p>
      <p className={`mt-1 text-2xl font-bold ${color ?? 'text-gray-900'}`}>{value}</p>
      {sub && <p className="mt-0.5 text-xs text-gray-400">{sub}</p>}
    </div>
  )
}

// Sparkline renders an inline SVG sparkline chart from numeric values.
function Sparkline({ data, color = '#3b82f6', height = 32 }: { data: number[]; color?: string; height?: number }) {
  if (data.length < 2) return null
  const width = 120
  const max = Math.max(...data)
  const min = Math.min(...data)
  const range = max - min || 1
  const points = data
    .map((v, i) => {
      const x = (i / (data.length - 1)) * width
      const y = height - ((v - min) / range) * (height - 4) - 2
      return `${x},${y}`
    })
    .join(' ')

  return (
    <svg width={width} height={height} className="inline-block">
      <polyline fill="none" stroke={color} strokeWidth="1.5" points={points} />
    </svg>
  )
}

// --- Issue Summary ---

function IssueSummary({ issues }: { issues: Issue[] }) {
  const openCount = issues.filter((i) => i.state === 'open').length
  const closedCount = issues.filter((i) => i.state === 'closed').length

  const queued = issues.filter((i) => i.state === 'open' && i.labels.includes('status:queued')).length
  const inProgress = issues.filter((i) => i.state === 'open' && i.labels.includes('status:in-progress')).length
  const blocked = issues.filter((i) => i.state === 'open' && i.labels.includes('status:blocked')).length
  const needsHuman = issues.filter((i) =>
    i.state === 'open' && i.labels.some((l) => ['agent:human', 'status:needs-human', 'status:human-pending'].includes(l))
  ).length

  // Closed in last 24h / 7d
  const now = Date.now()
  const closed24h = issues.filter((i) => i.closed_at && now - new Date(i.closed_at).getTime() < 86_400_000).length
  const closed7d = issues.filter((i) => i.closed_at && now - new Date(i.closed_at).getTime() < 7 * 86_400_000).length

  return (
    <SectionCard title="Issues">
      <div className="grid grid-cols-2 gap-3 py-2">
        <div>
          <MetricRow label="Total Open" value={openCount.toString()} />
          <MetricRow label="Total Closed" value={closedCount.toString()} />
          <MetricRow label="Closed (24h)" value={closed24h.toString()} />
          <MetricRow label="Closed (7d)" value={closed7d.toString()} />
        </div>
        <div>
          <MetricRow label="Queued" value={queued.toString()} />
          <MetricRow label="In Progress" value={inProgress.toString()} />
          <MetricRow label="Blocked" value={blocked.toString()} />
          <MetricRow label="Needs Human" value={needsHuman.toString()} />
        </div>
      </div>
    </SectionCard>
  )
}

// --- Activity Feed ---

function ActivityFeed({ sessions }: { sessions: Session[] }) {
  const recent = useMemo(() => {
    return [...sessions]
      .sort((a, b) => new Date(b.started_at).getTime() - new Date(a.started_at).getTime())
      .slice(0, 15)
  }, [sessions])

  const statusColor: Record<string, string> = {
    completed: 'bg-green-100 text-green-700',
    running: 'bg-blue-100 text-blue-700',
    failed: 'bg-red-100 text-red-700',
    timeout: 'bg-orange-100 text-orange-700',
  }

  return (
    <SectionCard title="Recent Activity">
      {recent.length === 0 ? (
        <div className="py-6 text-center text-sm text-gray-400">No sessions recorded</div>
      ) : (
        <div className="max-h-80 overflow-auto">
          {recent.map((s) => (
            <div key={s.id} className="flex items-center gap-3 border-b py-2 last:border-b-0">
              <span className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${statusColor[s.status] ?? 'bg-gray-100 text-gray-600'}`}>
                {s.status}
              </span>
              <span className="text-sm text-gray-900 font-medium">#{s.issue_number}</span>
              <span className="text-xs text-gray-500">{s.agent_type}</span>
              <span className="text-xs text-gray-400 font-mono">{s.provider}</span>
              <span className="ml-auto text-xs text-gray-400 whitespace-nowrap">{formatRelative(s.started_at)}</span>
            </div>
          ))}
        </div>
      )}
    </SectionCard>
  )
}

// --- Trending Sparklines ---

function TrendingSection({ entries }: { entries: HistoryEntry[] }) {
  if (entries.length < 2) {
    return (
      <SectionCard title="Trending (24h)">
        <div className="py-6 text-center text-sm text-gray-400">Collecting data...</div>
      </SectionCard>
    )
  }

  const workers = entries.map((e) => e.pool?.active_workers ?? 0)
  const queue = entries.map((e) => e.pool?.queue_depth ?? 0)
  const goroutines = entries.map((e) => e.system?.goroutines ?? 0)
  const heap = entries.map((e) => (e.system?.heap_alloc_bytes ?? 0) / 1_048_576)
  const latency = entries.map((e) => e.dispatcher?.avg_poll_latency_ms ?? 0)
  const events = entries.map((e) => e.dispatcher?.total_events_processed ?? 0)
  // Compute deltas for events (cumulative counter)
  const eventDeltas = events.slice(1).map((v, i) => Math.max(0, v - events[i]))

  const rows: { label: string; data: number[]; color: string; current: string }[] = [
    { label: 'Active Workers', data: workers, color: '#3b82f6', current: workers[workers.length - 1].toString() },
    { label: 'Queue Depth', data: queue, color: '#f59e0b', current: queue[queue.length - 1].toString() },
    { label: 'Goroutines', data: goroutines, color: '#8b5cf6', current: goroutines[goroutines.length - 1].toString() },
    { label: 'Heap (MB)', data: heap, color: '#10b981', current: heap[heap.length - 1].toFixed(1) },
    { label: 'Poll Latency (ms)', data: latency, color: '#ef4444', current: latency[latency.length - 1].toFixed(1) },
    { label: 'Events/interval', data: eventDeltas, color: '#06b6d4', current: eventDeltas.length > 0 ? eventDeltas[eventDeltas.length - 1].toString() : '0' },
  ]

  return (
    <SectionCard title="Trending (24h)">
      <div className="space-y-1 py-1">
        {rows.map((r) => (
          <div key={r.label} className="flex items-center justify-between py-1.5 border-b last:border-b-0">
            <span className="text-sm text-gray-600 w-36">{r.label}</span>
            <Sparkline data={r.data} color={r.color} />
            <span className="text-sm font-medium text-gray-900 font-mono w-16 text-right">{r.current}</span>
          </div>
        ))}
      </div>
    </SectionCard>
  )
}

// --- Existing Section Components (unchanged) ---

const pressureColors: Record<string, string> = {
  low: 'bg-green-50 border-green-200 text-green-800',
  moderate: 'bg-yellow-50 border-yellow-200 text-yellow-800',
  high: 'bg-orange-50 border-orange-200 text-orange-800',
  critical: 'bg-red-50 border-red-200 text-red-800',
}

function PressureSection({ pressure }: { pressure: PressureMetrics }) {
  const colors = pressureColors[pressure.level] ?? pressureColors.low
  return (
    <div className={`rounded-lg border px-4 py-3 ${colors}`}>
      <div className="flex items-center gap-2">
        <span className="text-sm font-semibold uppercase tracking-wide">
          Pressure: {pressure.level}
        </span>
      </div>
      {pressure.reasons != null && pressure.reasons.length > 0 && (
        <ul className="mt-1 list-disc pl-5 text-xs">
          {pressure.reasons.map((r) => (
            <li key={r}>{r}</li>
          ))}
        </ul>
      )}
    </div>
  )
}

function PoolSection({ pool }: { pool: PoolMetrics | null }) {
  if (pool == null) return <NullSection title="Agent Pool" />

  const utilization =
    pool.total_workers > 0 ? Math.round((pool.active_workers / pool.total_workers) * 100) : 0

  return (
    <SectionCard title="Agent Pool">
      <MetricRow label="Total Workers" value={pool.total_workers.toString()} tooltip="Current number of worker goroutines in the agent pool." />
      <MetricRow label="Active Workers" value={pool.active_workers.toString()} tooltip="Workers currently executing a task." />
      <MetricRow label="Idle Workers" value={pool.idle_workers.toString()} tooltip="Workers waiting for tasks." />
      <MetricRow label="Utilization" value={`${utilization}%`} tooltip="Percentage of workers currently active." />
      <MetricRow label="Queue Depth" value={pool.queue_depth.toString()} tooltip="Tasks waiting for an available worker." />
      <MetricRow label="Tasks Completed" value={pool.tasks_completed.toLocaleString()} tooltip="Total tasks successfully completed since startup." />
      <MetricRow label="Tasks Failed" value={pool.tasks_failed.toLocaleString()} tooltip="Total tasks that failed. Failed tasks may be requeued." />
      <MetricRow label="Avg Task Duration" value={formatMs(pool.avg_task_duration_ms)} tooltip="Average wall-clock time per completed task." />
      <MetricRow label="P95 Task Duration" value={formatMs(pool.p95_task_duration_ms)} tooltip="95th percentile task duration." />
    </SectionCard>
  )
}

function DispatcherSection({ dispatcher }: { dispatcher: DispatcherMetrics | null }) {
  if (dispatcher == null) return <NullSection title="Dispatcher" />

  const lastPoll =
    dispatcher.last_poll_at === '0001-01-01T00:00:00Z'
      ? 'Never'
      : new Date(dispatcher.last_poll_at).toLocaleString()

  return (
    <SectionCard title="Dispatcher">
      <MetricRow label="Claimed Issues" value={dispatcher.claimed_count.toString()} tooltip="Issues currently claimed for processing." />
      <MetricRow label="Total Routed" value={dispatcher.total_routed.toLocaleString()} tooltip="Total issues routed since startup." />
      <MetricRow label="Total Requeued" value={dispatcher.total_requeued.toLocaleString()} tooltip="Issues returned to queue for retry." />
      <MetricRow label="Events Processed" value={dispatcher.total_events_processed.toLocaleString()} tooltip="Total forge events processed." />
      <MetricRow label="Avg Poll Latency" value={formatMs(dispatcher.avg_poll_latency_ms)} tooltip="Average time to poll the forge." />
      <MetricRow label="Last Poll" value={lastPoll} tooltip="Most recent forge poll timestamp." />
    </SectionCard>
  )
}

function SystemSection({ system }: { system: SystemMetrics | null }) {
  if (system == null) return <NullSection title="System" />

  return (
    <SectionCard title="System">
      <MetricRow label="Goroutines" value={system.goroutines.toString()} />
      <MetricRow label="Heap Allocated" value={formatBytes(system.heap_alloc_bytes)} />
      <MetricRow label="OS Memory" value={formatBytes(system.sys_bytes_total)} />
      <MetricRow label="GC Cycles" value={system.gc_cycles.toString()} />
      <MetricRow label="Next GC Target" value={formatBytes(system.next_gc_bytes)} />
    </SectionCard>
  )
}

function ScalingSection({
  events,
  config,
}: {
  events: ScalingEvent[] | null
  config: ScalingConfig | null
}) {
  if (config == null) return <NullSection title="Adaptive Scaling" />

  return (
    <SectionCard title="Adaptive Scaling">
      <MetricRow label="Status" value={config.enabled ? 'Enabled' : 'Disabled'} />
      <MetricRow label="Min Workers" value={config.min_workers.toString()} />
      <MetricRow label="Max Workers" value={config.max_workers.toString()} />
      <MetricRow label="Current Target" value={config.current_target.toString()} />
      {events != null && events.length > 0 && (
        <div className="mt-3">
          <p className="mb-2 text-xs font-semibold text-gray-500 uppercase tracking-wide">
            Recent Events
          </p>
          <div className="space-y-1">
            {events.map((e) => (
              <div key={e.timestamp} className="rounded bg-gray-50 px-3 py-2 text-xs">
                <div className="flex items-center justify-between">
                  <span
                    className={
                      e.action === 'scale-up'
                        ? 'font-semibold text-green-700'
                        : 'font-semibold text-amber-700'
                    }
                  >
                    {e.action === 'scale-up' ? '▲' : '▼'} {e.from_workers}→{e.to_workers} workers
                  </span>
                  <span className="text-gray-400">
                    {Math.round(e.confidence * 100)}% conf
                  </span>
                </div>
                <p className="mt-0.5 text-gray-500 truncate">{e.reason}</p>
                <p className="text-gray-400">{new Date(e.timestamp).toLocaleString()}</p>
              </div>
            ))}
          </div>
        </div>
      )}
      {(events == null || events.length === 0) && (
        <div className="py-4 text-center text-xs text-gray-400">No scaling events yet</div>
      )}
    </SectionCard>
  )
}

function CapacitySection({ capacity, maxWorkers }: { capacity: CapacityInfo | null; maxWorkers: number }) {
  if (capacity == null) return <NullSection title="Capacity" />

  const healthyCount = capacity.providers.filter((p) => p.healthy).length
  const routingChainNames = Object.keys(capacity.routing_chains)

  return (
    <SectionCard title="Capacity">
      <MetricRow label="AI Providers" value={`${healthyCount} / ${capacity.providers.length}`} />
      <MetricRow label="Max Concurrent Sessions" value={maxWorkers.toString()} />
      <MetricRow label="Routing Chains" value={routingChainNames.length.toString()} />
      {capacity.providers.length > 0 && (
        <div className="mt-3">
          <p className="mb-2 text-xs font-semibold text-gray-500 uppercase tracking-wide">
            Providers
          </p>
          <div className="space-y-1">
            {capacity.providers.map((p) => (
              <div key={p.name} className="flex items-center justify-between rounded bg-gray-50 px-3 py-2 text-xs">
                <div className="flex items-center gap-2">
                  <span className={`inline-block h-2 w-2 rounded-full ${p.healthy ? 'bg-green-500' : 'bg-red-400'}`} />
                  <span className="font-medium text-gray-700">{p.name}</span>
                </div>
                <span className="text-gray-500 font-mono">{p.model}</span>
              </div>
            ))}
          </div>
        </div>
      )}
      {routingChainNames.length > 0 && (
        <div className="mt-3">
          <p className="mb-2 text-xs font-semibold text-gray-500 uppercase tracking-wide">
            Routing
          </p>
          <div className="space-y-1">
            {routingChainNames.map((chain) => (
              <div key={chain} className="flex items-center justify-between rounded bg-gray-50 px-3 py-2 text-xs">
                <span className="font-medium text-gray-700">{chain}</span>
                <span className="text-gray-500 font-mono">{capacity.routing_chains[chain].join(' → ')}</span>
              </div>
            ))}
          </div>
        </div>
      )}
    </SectionCard>
  )
}

// --- Main Page ---

export function Metrics() {
  const metrics = useQuery({
    queryKey: ['metrics'],
    queryFn: api.getMetrics,
    refetchInterval: 5_000,
  })

  const history = useQuery({
    queryKey: ['metrics-history'],
    queryFn: () => api.getMetricsHistory('24h'),
    refetchInterval: 30_000,
  })

  const issues = useQuery({
    queryKey: ['issues', 'all-for-metrics'],
    queryFn: async () => {
      const [open, closed] = await Promise.all([
        api.listIssues({ state: 'open' }),
        api.listIssues({ state: 'closed' }),
      ])
      return [...open, ...closed]
    },
    refetchInterval: 60_000,
  })

  const sessions = useQuery({
    queryKey: ['sessions'],
    queryFn: api.listSessions,
    refetchInterval: 15_000,
  })

  const costs = useQuery({
    queryKey: ['costs'],
    queryFn: api.getCosts,
    refetchInterval: 30_000,
  })

  // Compute top-line stats
  const openCount = issues.data?.filter((i) => i.state === 'open').length ?? 0
  const closedCount = issues.data?.filter((i) => i.state === 'closed').length ?? 0
  const totalSessions = sessions.data?.length ?? 0
  const completedSessions = sessions.data?.filter((s) => s.status === 'completed').length ?? 0
  const costUsd = costs.data?.total_cost_usd ?? 0

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h2 className="text-2xl font-bold text-gray-900">Metrics</h2>
        <span className="text-xs text-gray-400">Auto-refreshes every 5s</span>
      </div>

      {metrics.isLoading && (
        <div className="flex items-center justify-center py-20">
          <p className="text-gray-500">Loading metrics...</p>
        </div>
      )}

      {metrics.isError && (
        <div className="rounded-lg border border-red-200 bg-red-50 p-4">
          <p className="font-medium text-red-800">Failed to load metrics</p>
          <p className="mt-1 text-sm text-red-600">{metrics.error?.message ?? 'Unknown error'}</p>
        </div>
      )}

      {metrics.data != null && (
        <div className="space-y-6">
          {/* Top-line stats */}
          <div className="grid grid-cols-2 gap-4 lg:grid-cols-5">
            <StatCard label="Open Issues" value={openCount.toString()} color="text-blue-700" sub={`${closedCount} closed`} />
            <StatCard label="Sessions" value={totalSessions.toString()} sub={`${completedSessions} completed`} />
            <StatCard
              label="Tasks Completed"
              value={(metrics.data.pool?.tasks_completed ?? 0).toLocaleString()}
              sub={`${metrics.data.pool?.tasks_failed ?? 0} failed`}
            />
            <StatCard
              label="Events Processed"
              value={(metrics.data.dispatcher?.total_events_processed ?? 0).toLocaleString()}
              sub={`${metrics.data.dispatcher?.total_routed ?? 0} routed`}
            />
            <StatCard
              label="Total Cost"
              value={`$${costUsd.toFixed(2)}`}
              sub={`${(costs.data?.total_input_tokens ?? 0).toLocaleString()} tokens`}
            />
          </div>

          <PressureSection pressure={metrics.data.pressure} />

          {/* Trending + Activity + Issues */}
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
            <TrendingSection entries={history.data?.entries ?? []} />
            {sessions.data != null && <ActivityFeed sessions={sessions.data} />}
            {issues.data != null && <IssueSummary issues={issues.data} />}
          </div>

          {/* Core metrics */}
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
            <PoolSection pool={metrics.data.pool} />
            <DispatcherSection dispatcher={metrics.data.dispatcher} />
            <SystemSection system={metrics.data.system} />
          </div>

          {/* Scaling + Capacity */}
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            <CapacitySection
              capacity={metrics.data.capacity}
              maxWorkers={metrics.data.scaling_config?.max_workers ?? metrics.data.pool?.total_workers ?? 0}
            />
            <ScalingSection
              events={metrics.data.scaling_events}
              config={metrics.data.scaling_config}
            />
          </div>
        </div>
      )}
    </div>
  )
}
