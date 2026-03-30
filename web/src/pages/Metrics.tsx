import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import type {
  PoolMetrics, DispatcherMetrics, SystemMetrics, PressureMetrics,
  ScalingEvent, ScalingConfig, CapacityInfo, Issue, Session, HistoryEntry,
  HostMetricsDTO,
} from '../lib/api'

function formatBytes(bytes: number): string {
  if (bytes >= 1_073_741_824) return `${(bytes / 1_073_741_824).toFixed(1)} GB`
  if (bytes >= 1_048_576) return `${(bytes / 1_048_576).toFixed(1)} MB`
  if (bytes >= 1_024) return `${(bytes / 1_024).toFixed(1)} KB`
  return `${bytes} B`
}

function formatMs(ms: number): string {
  if (ms === 0) return '\u2014'
  if (ms >= 1_000) return `${(ms / 1_000).toFixed(2)}s`
  return `${ms.toFixed(1)}ms`
}

function formatRelative(dateStr: string): string {
  const parsed = new Date(dateStr).getTime()
  if (isNaN(parsed)) return 'unknown'
  const diffMs = Date.now() - parsed
  if (diffMs < 1000) return 'just now'
  const m = Math.floor(diffMs / 60_000)
  const h = Math.floor(diffMs / 3_600_000)
  const d = Math.floor(diffMs / 86_400_000)
  if (m < 1) return 'just now'
  if (m < 60) return `${m}m ago`
  if (h < 24) return `${h}h ago`
  if (d < 30) return `${d}d ago`
  return new Date(dateStr).toLocaleDateString()
}

function formatBytesHost(bytes: number): string {
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`
}

function estimateDaysToThreshold(history: HostMetricsDTO[], currentPct: number, thresholdPct: number): string {
  if (history.length < 2 || currentPct >= thresholdPct) return ''
  const first = history[0]
  const last = history[history.length - 1]
  const firstTime = new Date(first.collected_at).getTime()
  const lastTime = new Date(last.collected_at).getTime()
  const daysDiff = (lastTime - firstTime) / (1000 * 60 * 60 * 24)
  if (daysDiff < 0.01) return ''
  const pctDiff = last.disk_percent - first.disk_percent
  const dailyRate = pctDiff / daysDiff
  if (dailyRate <= 0.001) return 'stable'
  const daysToThreshold = (thresholdPct - currentPct) / dailyRate
  return `~${Math.round(daysToThreshold)} days to ${thresholdPct}%`
}

function GaugeCard({ label, percent, usedLabel, totalLabel, warnPct, critPct }: {
  label: string
  percent: number
  usedLabel: string
  totalLabel: string
  warnPct: number
  critPct: number
}) {
  const color = percent >= critPct ? 'text-red-600' : percent >= warnPct ? 'text-amber-600' : 'text-green-600'
  const bgColor = percent >= critPct
    ? 'bg-red-100 dark:bg-red-900/30'
    : percent >= warnPct
      ? 'bg-amber-100 dark:bg-amber-900/30'
      : 'bg-green-100 dark:bg-green-900/30'
  const barColor = percent >= critPct ? 'bg-red-500' : percent >= warnPct ? 'bg-amber-500' : 'bg-green-500'

  return (
    <div className={`rounded-lg border dark:border-gray-700 p-4 ${bgColor}`}>
      <div className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">{label}</div>
      <div className={`text-2xl font-bold ${color}`}>{percent.toFixed(1)}%</div>
      <div className="mt-2 h-2 rounded-full bg-gray-200 dark:bg-gray-700">
        <div className={`h-2 rounded-full ${barColor}`} style={{ width: `${Math.min(percent, 100)}%` }} />
      </div>
      <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">{usedLabel} / {totalLabel}</div>
    </div>
  )
}

function HostSection({ current, history }: { current: HostMetricsDTO; history: HostMetricsDTO[] }) {
  const cpuPercent = current.cpu_percent ?? (current.num_cpu > 0 ? (current.load_avg_1 / current.num_cpu) * 100 : 0)
  const diskTrend = estimateDaysToThreshold(history, current.disk_percent, 70)

  return (
    <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
      <h3 className="mb-3 text-sm font-semibold text-gray-900 dark:text-gray-100">Host Resources</h3>
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <GaugeCard
          label="Disk"
          percent={current.disk_percent}
          usedLabel={formatBytesHost(current.disk_used_bytes)}
          totalLabel={formatBytesHost(current.disk_total_bytes)}
          warnPct={70}
          critPct={85}
        />
        <GaugeCard
          label="RAM"
          percent={current.ram_percent}
          usedLabel={formatBytesHost(current.ram_used_bytes)}
          totalLabel={formatBytesHost(current.ram_total_bytes)}
          warnPct={80}
          critPct={90}
        />
        <GaugeCard
          label="Swap"
          percent={current.swap_percent}
          usedLabel={formatBytesHost(current.swap_used_bytes)}
          totalLabel={formatBytesHost(current.swap_total_bytes)}
          warnPct={25}
          critPct={50}
        />
        <GaugeCard
          label={current.in_lxc ? 'CPU (cgroup)' : 'CPU Load'}
          percent={cpuPercent}
          usedLabel={`${cpuPercent.toFixed(1)}%`}
          totalLabel={`${current.num_cpu} cores`}
          warnPct={70}
          critPct={90}
        />
      </div>
      {current.alerts != null && current.alerts.length > 0 && (
        <div className="mt-3 space-y-1">
          {current.alerts.map((a, i) => (
            <div
              key={i}
              className={`rounded px-3 py-1 text-xs ${a.level === 'critical' ? 'bg-red-100 dark:bg-red-900/30 text-red-800 dark:text-red-300' : 'bg-amber-100 dark:bg-amber-900/30 text-amber-800 dark:text-amber-300'}`}
            >
              {a.message}
            </div>
          ))}
        </div>
      )}
      {history.length > 1 && (
        <div className="mt-2 text-xs text-gray-500 dark:text-gray-400">
          Disk trend: {diskTrend || 'stable'}
        </div>
      )}
    </div>
  )
}

function MetricRow({ label, value, tooltip, sparkline }: { label: string; value: string; tooltip?: string; sparkline?: number[] }) {
  return (
    <div className="flex items-center gap-2 py-2 border-b dark:border-gray-700 last:border-b-0">
      {tooltip ? (
        <span className="group relative shrink-0 text-sm text-gray-600 dark:text-gray-400 cursor-help underline decoration-dotted decoration-gray-400 dark:decoration-gray-600 underline-offset-2">
          {label}
          <span className="pointer-events-none absolute left-0 bottom-full mb-1 z-10 hidden w-64 rounded bg-gray-900 dark:bg-gray-700 px-3 py-2 text-xs leading-relaxed text-gray-100 shadow-lg group-hover:block">
            {tooltip}
          </span>
        </span>
      ) : (
        <span className="shrink-0 text-sm text-gray-600 dark:text-gray-400">{label}</span>
      )}
      {sparkline != null && sparkline.length >= 2 && <Sparkline data={sparkline} />}
      {sparkline == null && <span className="flex-1" />}
      <span className="shrink-0 text-sm font-medium text-gray-900 dark:text-gray-100 font-mono text-right">{value}</span>
    </div>
  )
}

function SectionCard({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800">
      <div className="px-4 py-3 border-b dark:border-gray-700 bg-gray-50 dark:bg-gray-900">
        <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300">{title}</h3>
      </div>
      <div className="px-4 py-2">{children}</div>
    </div>
  )
}

function NullSection({ title }: { title: string }) {
  return (
    <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800">
      <div className="px-4 py-3 border-b dark:border-gray-700 bg-gray-50 dark:bg-gray-900">
        <h3 className="text-sm font-semibold text-gray-400 dark:text-gray-500">{title}</h3>
      </div>
      <div className="px-4 py-6 text-center text-sm text-gray-400 dark:text-gray-500">Not running</div>
    </div>
  )
}

function StatCard({ label, value, sub, color }: { label: string; value: string; sub?: string; color?: string }) {
  return (
    <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 px-4 py-3">
      <p className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wide">{label}</p>
      <p className={`mt-1 text-2xl font-bold ${color ?? 'text-gray-900 dark:text-gray-100'}`}>{value}</p>
      {sub && <p className="mt-0.5 text-xs text-gray-400 dark:text-gray-500">{sub}</p>}
    </div>
  )
}

function Sparkline({ data, color = '#4ade80' }: { data: number[]; color?: string }) {
  if (data.length < 2) return null
  const w = 80
  const h = 24
  const min = Math.min(...data)
  const max = Math.max(...data)
  const range = max - min || 1

  const pts = data.map((v, i) => ({
    x: (i / (data.length - 1)) * w,
    y: h - ((v - min) / range) * (h - 4) - 2,
  }))

  // Build smooth SVG path using cubic bezier curves (GitHub style)
  let d = `M ${pts[0].x} ${pts[0].y}`
  for (let i = 1; i < pts.length; i++) {
    const prev = pts[i - 1]
    const curr = pts[i]
    const cpx = (prev.x + curr.x) / 2
    d += ` C ${cpx} ${prev.y}, ${cpx} ${curr.y}, ${curr.x} ${curr.y}`
  }

  // Area fill path (close back along bottom)
  const fill = `${d} L ${pts[pts.length - 1].x} ${h} L ${pts[0].x} ${h} Z`

  return (
    <svg width={w} height={h} className="shrink-0 overflow-visible">
      <defs>
        <linearGradient id="spark-fill" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={color} stopOpacity="0.3" />
          <stop offset="100%" stopColor={color} stopOpacity="0.02" />
        </linearGradient>
      </defs>
      <path d={fill} fill="url(#spark-fill)" stroke="none" />
      <path d={d} fill="none" stroke={color} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
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

  // Closed in last 24h / 7d (stable snapshot per render cycle)
  // eslint-disable-next-line react-hooks/purity -- Date.now() in a memoized time-window calculation is intentional
  const now = useMemo(() => Date.now(), [])
  const closed24h = issues.filter((i) => i.closed_at && now - new Date(i.closed_at).getTime() < 86_400_000).length
  const closed7d = issues.filter((i) => i.closed_at && now - new Date(i.closed_at).getTime() < 7 * 86_400_000).length

  return (
    <SectionCard title="Issues">
      <div className="grid grid-cols-2 gap-3 py-2">
        <div>
          <MetricRow label="Total Open" value={openCount.toString()} tooltip="All open issues across all pipeline stages." />
          <MetricRow label="Total Closed" value={closedCount.toString()} tooltip="All closed/completed issues." />
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
    completed: 'bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-300',
    running: 'bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300',
    failed: 'bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-300',
    timeout: 'bg-orange-100 dark:bg-orange-900/30 text-orange-700 dark:text-orange-300',
  }

  return (
    <SectionCard title="Recent Activity">
      {recent.length === 0 ? (
        <div className="py-6 text-center text-sm text-gray-400 dark:text-gray-500">No sessions recorded</div>
      ) : (
        <div className="max-h-80 overflow-auto">
          {recent.map((s) => (
            <div key={s.id} className="flex items-center gap-3 border-b dark:border-gray-700 py-2 last:border-b-0">
              <span className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${statusColor[s.status] ?? 'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300'}`}>
                {s.status}
              </span>
              <span className="text-sm font-medium text-blue-600 dark:text-blue-400">IS#{s.issue_number}</span>
              <span className="text-xs text-gray-500 dark:text-gray-400">{s.agent_type}</span>
              <span className="text-xs text-gray-400 dark:text-gray-500 font-mono">{s.provider}</span>
              <span className="ml-auto text-xs text-gray-400 dark:text-gray-500 whitespace-nowrap">{formatRelative(s.started_at)}</span>
            </div>
          ))}
        </div>
      )}
    </SectionCard>
  )
}

// --- Section Components ---

const pressureColors: Record<string, string> = {
  low: 'bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800 text-green-800 dark:text-green-300',
  moderate: 'bg-yellow-50 dark:bg-yellow-900/20 border-yellow-200 dark:border-yellow-800 text-yellow-800 dark:text-yellow-300',
  high: 'bg-orange-50 dark:bg-orange-900/20 border-orange-200 dark:border-orange-800 text-orange-800 dark:text-orange-300',
  critical: 'bg-red-50 dark:bg-red-900/20 border-red-200 dark:border-red-800 text-red-800 dark:text-red-300',
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

function PoolSection({ pool, history }: { pool: PoolMetrics | null; history: HistoryEntry[] }) {
  if (pool == null) return <NullSection title="Agent Pool" />

  const utilization =
    pool.total_workers > 0 ? Math.round((pool.active_workers / pool.total_workers) * 100) : 0

  const workerSparkline = history.map((e) => e.pool?.active_workers ?? 0)
  const idleSparkline = history.map((e) => e.pool?.idle_workers ?? 0)
  const queueSparkline = history.map((e) => e.pool?.queue_depth ?? 0)
  const completedSparkline = history.map((e) => e.pool?.tasks_completed ?? 0)
  const failedSparkline = history.map((e) => e.pool?.tasks_failed ?? 0)

  return (
    <SectionCard title="Agent Pool">
      <MetricRow label="Total Workers" value={pool.total_workers.toString()} tooltip="Current number of worker goroutines in the agent pool." />
      <MetricRow label="Active Workers" value={pool.active_workers.toString()} tooltip="Workers currently executing a task." sparkline={workerSparkline} />
      <MetricRow label="Idle Workers" value={pool.idle_workers.toString()} tooltip="Workers waiting for tasks." sparkline={idleSparkline} />
      <MetricRow label="Utilization" value={`${utilization}%`} tooltip="Percentage of workers currently active." />
      <MetricRow label="Queue Depth" value={pool.queue_depth.toString()} tooltip="Tasks waiting for an available worker." sparkline={queueSparkline} />
      <MetricRow label="Tasks Completed" value={pool.tasks_completed.toLocaleString()} tooltip="Total tasks successfully completed since startup." sparkline={completedSparkline} />
      <MetricRow label="Tasks Failed" value={pool.tasks_failed.toLocaleString()} tooltip="Total tasks that failed. Failed tasks may be requeued." sparkline={failedSparkline} />
      <MetricRow label="Avg Task Duration" value={formatMs(pool.avg_task_duration_ms)} tooltip="Average wall-clock time per completed task." />
      <MetricRow label="P95 Task Duration" value={formatMs(pool.p95_task_duration_ms)} tooltip="95th percentile task duration." />
    </SectionCard>
  )
}

function DispatcherSection({ dispatcher, history }: { dispatcher: DispatcherMetrics | null; history: HistoryEntry[] }) {
  if (dispatcher == null) return <NullSection title="Dispatcher" />

  const lastPoll =
    dispatcher.last_poll_at === '0001-01-01T00:00:00Z'
      ? 'Never'
      : new Date(dispatcher.last_poll_at).toLocaleString()

  const claimedSparkline = history.map((e) => e.dispatcher?.claimed_count ?? 0)
  const routedSparkline = history.map((e) => e.dispatcher?.total_routed ?? 0)
  const requeuedSparkline = history.map((e) => e.dispatcher?.total_requeued ?? 0)
  const latencySparkline = history.map((e) => e.dispatcher?.avg_poll_latency_ms ?? 0)
  const events = history.map((e) => e.dispatcher?.total_events_processed ?? 0)
  const eventDeltaSparkline = events.slice(1).map((v, i) => Math.max(0, v - events[i]))

  return (
    <SectionCard title="Dispatcher">
      <MetricRow label="Claimed Issues" value={dispatcher.claimed_count.toString()} tooltip="Issues currently claimed for processing." sparkline={claimedSparkline} />
      <MetricRow label="Total Routed" value={dispatcher.total_routed.toLocaleString()} tooltip="Total issues routed since startup." sparkline={routedSparkline} />
      <MetricRow label="Total Requeued" value={dispatcher.total_requeued.toLocaleString()} tooltip="Issues returned to queue for retry." sparkline={requeuedSparkline} />
      <MetricRow label="Events Processed" value={dispatcher.total_events_processed.toLocaleString()} tooltip="Total forge events processed." sparkline={eventDeltaSparkline} />
      <MetricRow label="Avg Poll Latency" value={formatMs(dispatcher.avg_poll_latency_ms)} tooltip="Average time to poll the forge." sparkline={latencySparkline} />
      <MetricRow label="Last Poll" value={lastPoll} tooltip="Most recent forge poll timestamp." />
    </SectionCard>
  )
}

function SystemSection({ system, history }: { system: SystemMetrics | null; history: HistoryEntry[] }) {
  if (system == null) return <NullSection title="System" />

  const goroutineSparkline = history.map((e) => e.system?.goroutines ?? 0)
  const heapSparkline = history.map((e) => e.system?.heap_alloc_bytes ?? 0)
  const osMemSparkline = history.map((e) => e.system?.sys_bytes_total ?? 0)
  const gcSparkline = history.map((e) => e.system?.gc_cycles ?? 0)

  return (
    <SectionCard title="System">
      <MetricRow label="Goroutines" value={system.goroutines.toString()} sparkline={goroutineSparkline} />
      <MetricRow label="Heap Allocated" value={formatBytes(system.heap_alloc_bytes)} sparkline={heapSparkline} />
      <MetricRow label="OS Memory" value={formatBytes(system.sys_bytes_total)} sparkline={osMemSparkline} />
      <MetricRow label="GC Cycles" value={system.gc_cycles.toString()} sparkline={gcSparkline} />
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
          <p className="mb-2 text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wide">
            Recent Events
          </p>
          <div className="space-y-1">
            {events.map((e) => (
              <div key={e.timestamp} className="rounded bg-gray-50 dark:bg-gray-900 px-3 py-2 text-xs">
                <div className="flex items-center justify-between">
                  <span
                    className={
                      e.action === 'scale-up'
                        ? 'font-semibold text-green-700 dark:text-green-400'
                        : 'font-semibold text-amber-700 dark:text-amber-400'
                    }
                  >
                    {e.action === 'scale-up' ? '\u25B2' : '\u25BC'} {e.from_workers}\u2192{e.to_workers} workers
                  </span>
                  <span className="text-gray-400 dark:text-gray-500">
                    {Math.round(e.confidence * 100)}% conf
                  </span>
                </div>
                <p className="mt-0.5 text-gray-500 dark:text-gray-400 truncate">{e.reason}</p>
                <p className="text-gray-400 dark:text-gray-500">{new Date(e.timestamp).toLocaleString()}</p>
              </div>
            ))}
          </div>
        </div>
      )}
      {(events == null || events.length === 0) && (
        <div className="py-4 text-center text-xs text-gray-400 dark:text-gray-500">No scaling events yet</div>
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
          <p className="mb-2 text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wide">
            Providers
          </p>
          <div className="space-y-1">
            {capacity.providers.map((p) => (
              <div key={p.name} className="flex items-center justify-between rounded bg-gray-50 dark:bg-gray-900 px-3 py-2 text-xs">
                <div className="flex items-center gap-2">
                  <span className={`inline-block h-2 w-2 rounded-full ${p.healthy ? 'bg-green-500' : 'bg-red-400'}`} />
                  <span className="font-medium text-gray-700 dark:text-gray-300">{p.name}</span>
                </div>
                <span className="text-gray-500 dark:text-gray-400 font-mono">{p.model}</span>
              </div>
            ))}
          </div>
        </div>
      )}
      {routingChainNames.length > 0 && (
        <div className="mt-3">
          <p className="mb-2 text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wide">
            Routing
          </p>
          <div className="space-y-1">
            {routingChainNames.map((chain) => (
              <div key={chain} className="flex items-center justify-between rounded bg-gray-50 dark:bg-gray-900 px-3 py-2 text-xs">
                <span className="font-medium text-gray-700 dark:text-gray-300">{chain}</span>
                <span className="text-gray-500 dark:text-gray-400 font-mono">{capacity.routing_chains[chain].join(' \u2192 ')}</span>
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

  const issueSummary = useQuery({
    queryKey: ['issue-summary'],
    queryFn: api.getIssueSummary,
    refetchInterval: 60_000,
  })

  // Full issue list for the detailed breakdown component (needs individual issues
  // for time-based filtering like "closed in 24h"). Top-line counts use issueSummary above.
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

  const hostMetrics = useQuery({
    queryKey: ['host-metrics'],
    queryFn: api.getHostMetrics,
    refetchInterval: 30_000,
  })

  // Compute top-line stats from cached summary (accurate counts).
  const openCount = issueSummary.data?.aggregate.open ?? 0
  const closedCount = issueSummary.data?.aggregate.closed ?? 0
  const totalSessions = sessions.data?.length ?? 0
  const completedSessions = sessions.data?.filter((s) => s.status === 'completed').length ?? 0
  const costUsd = costs.data?.estimated_cost_usd ?? 0

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h2 className="text-2xl font-bold text-gray-900 dark:text-gray-100">Metrics</h2>
        <span className="text-xs text-gray-400 dark:text-gray-500">Auto-refreshes every 5s</span>
      </div>

      {metrics.isLoading && (
        <div className="flex items-center justify-center py-20">
          <p className="text-gray-500 dark:text-gray-400">Loading metrics...</p>
        </div>
      )}

      {metrics.isError && (
        <div className="rounded-lg border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/30 p-4">
          <p className="font-medium text-red-800 dark:text-red-300">Failed to load metrics</p>
          <p className="mt-1 text-sm text-red-600 dark:text-red-400">{metrics.error?.message ?? 'Unknown error'}</p>
        </div>
      )}

      {metrics.data != null && (
        <div className="space-y-6">
          {/* Top-line stats */}
          <div className="grid grid-cols-2 gap-4 lg:grid-cols-5">
            <StatCard label="Total Open Issues" value={openCount.toString()} color="text-blue-700 dark:text-blue-400" sub={`all open across all stages · ${closedCount} closed`} />
            <StatCard label="Sessions" value={totalSessions.toString()} sub={`${completedSessions} completed`} />
            <StatCard
              label="Tasks Completed"
              value={(metrics.data.pool?.tasks_completed ?? 0).toLocaleString()}
              sub={`${metrics.data.pool?.tasks_failed ?? 0} pool task failures (since startup)`}
            />
            <StatCard
              label="Events Processed"
              value={(metrics.data.dispatcher?.total_events_processed ?? 0).toLocaleString()}
              sub={`${metrics.data.dispatcher?.total_routed ?? 0} routed`}
            />
            <StatCard
              label="Total Cost"
              value={`$${costUsd.toFixed(2)}`}
              sub={`${(costs.data?.tokens_used ?? 0).toLocaleString()} tokens`}
            />
          </div>

          <PressureSection pressure={metrics.data.pressure} />

          {/* Core metrics (with inline sparklines) */}
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
            <PoolSection pool={metrics.data.pool} history={history.data?.entries ?? []} />
            <DispatcherSection dispatcher={metrics.data.dispatcher} history={history.data?.entries ?? []} />
            <SystemSection system={metrics.data.system} history={history.data?.entries ?? []} />
          </div>

          {/* Activity + Issues */}
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            {sessions.data != null && <ActivityFeed sessions={sessions.data} />}
            {issues.data != null && <IssueSummary issues={issues.data} />}
          </div>

          {/* Host Resources */}
          {hostMetrics.data != null && (
            <HostSection
              current={hostMetrics.data.current}
              history={hostMetrics.data.history}
            />
          )}

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
