import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import type { PoolMetrics, DispatcherMetrics, SystemMetrics, PressureMetrics, ScalingEvent, ScalingConfig, CapacityInfo } from '../lib/api'

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
      <MetricRow label="Total Workers" value={pool.total_workers.toString()} tooltip="Current number of worker goroutines in the agent pool. Autoscaler adjusts this between min and max based on queue pressure." />
      <MetricRow label="Active Workers" value={pool.active_workers.toString()} tooltip="Workers currently executing a task. Each active worker is running an AI provider session." />
      <MetricRow label="Idle Workers" value={pool.idle_workers.toString()} tooltip="Workers waiting for tasks. Idle workers are ready to pick up work immediately." />
      <MetricRow label="Utilization" value={`${utilization}%`} tooltip="Percentage of workers currently active (active / total). High utilization with queued tasks triggers scale-up." />
      <MetricRow label="Queue Depth" value={pool.queue_depth.toString()} tooltip="Tasks waiting in the queue for an available worker. Non-zero means all workers are busy." />
      <MetricRow label="Tasks Completed" value={pool.tasks_completed.toLocaleString()} tooltip="Total tasks successfully completed since the dispatcher started." />
      <MetricRow label="Tasks Failed" value={pool.tasks_failed.toLocaleString()} tooltip="Total tasks that failed (provider error, timeout, budget exceeded). Failed tasks may be requeued." />
      <MetricRow label="Avg Task Duration" value={formatMs(pool.avg_task_duration_ms)} tooltip="Average wall-clock time per completed task, including provider API calls and issue updates." />
      <MetricRow label="P95 Task Duration" value={formatMs(pool.p95_task_duration_ms)} tooltip="95th percentile task duration. Tasks above this are unusually slow (complex issues or provider latency)." />
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
      <MetricRow label="Claimed Issues" value={dispatcher.claimed_count.toString()} tooltip="Issues currently claimed by the dispatcher via optimistic locking. These are in-flight or queued for processing." />
      <MetricRow label="Total Routed" value={dispatcher.total_routed.toLocaleString()} tooltip="Total issues routed to the agent pool since startup. Includes issues that were later requeued." />
      <MetricRow label="Total Requeued" value={dispatcher.total_requeued.toLocaleString()} tooltip="Issues that failed and were returned to the queue for retry. High counts may indicate provider issues." />
      <MetricRow label="Events Processed" value={dispatcher.total_events_processed.toLocaleString()} tooltip="Total forge events (issue opened, closed, labeled, commented) processed by the dispatcher." />
      <MetricRow label="Avg Poll Latency" value={formatMs(dispatcher.avg_poll_latency_ms)} tooltip="Average time to poll the forge for new events. High latency may indicate forge API rate limiting." />
      <MetricRow label="Last Poll" value={lastPoll} tooltip="Timestamp of the most recent forge poll. Polls occur at the configured interval (default 30s)." />
    </SectionCard>
  )
}

function SystemSection({ system }: { system: SystemMetrics | null }) {
  if (system == null) return <NullSection title="System" />

  return (
    <SectionCard title="System">
      <MetricRow label="Goroutines" value={system.goroutines.toString()} tooltip="Active Go goroutines. Each worker, the dispatcher, and HTTP handlers each use goroutines." />
      <MetricRow label="Heap Allocated" value={formatBytes(system.heap_alloc_bytes)} tooltip="Memory currently allocated on the Go heap. Rises during task execution and drops after GC." />
      <MetricRow label="OS Memory" value={formatBytes(system.sys_bytes_total)} tooltip="Total memory obtained from the OS. Includes heap, stacks, and GC metadata. This is the process RSS." />
      <MetricRow label="GC Cycles" value={system.gc_cycles.toString()} tooltip="Number of garbage collection cycles completed since process start." />
      <MetricRow label="Next GC Target" value={formatBytes(system.next_gc_bytes)} tooltip="Heap size that will trigger the next garbage collection cycle." />
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
      <MetricRow label="Status" value={config.enabled ? 'Enabled' : 'Disabled'} tooltip="Whether the autoscaler is actively adjusting worker count based on queue pressure." />
      <MetricRow label="Min Workers" value={config.min_workers.toString()} tooltip="Minimum worker count the autoscaler will maintain, even when idle." />
      <MetricRow label="Max Workers" value={config.max_workers.toString()} tooltip="Maximum worker count the autoscaler can scale to under high pressure." />
      <MetricRow label="Current Target" value={config.current_target.toString()} tooltip="The autoscaler's target worker count. The pool adjusts toward this value." />
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
      <MetricRow label="AI Providers" value={`${healthyCount} / ${capacity.providers.length}`} tooltip="Registered AI providers (healthy / total). Each provider represents a model that can execute agent tasks." />
      <MetricRow label="Max Concurrent Sessions" value={maxWorkers.toString()} tooltip="Maximum number of agent sessions that can run simultaneously, set by the autoscaler's max-workers limit." />
      <MetricRow label="Routing Chains" value={routingChainNames.length.toString()} tooltip="Number of routing chains (triage, default, complex, local, etc.). Each chain maps task types to a priority-ordered list of providers." />
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

export function Metrics() {
  const metrics = useQuery({
    queryKey: ['metrics'],
    queryFn: api.getMetrics,
    refetchInterval: 5_000,
  })

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
          <PressureSection pressure={metrics.data.pressure} />
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
            <PoolSection pool={metrics.data.pool} />
            <DispatcherSection dispatcher={metrics.data.dispatcher} />
            <SystemSection system={metrics.data.system} />
          </div>
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
