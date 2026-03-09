import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import type { PoolMetrics, DispatcherMetrics, SystemMetrics, ScalingEvent, ScalingConfig } from '../lib/api'

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

function MetricRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between py-2 border-b last:border-b-0">
      <span className="text-sm text-gray-600">{label}</span>
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

function PoolSection({ pool }: { pool: PoolMetrics | null }) {
  if (pool == null) return <NullSection title="Agent Pool" />

  const utilization =
    pool.total_workers > 0 ? Math.round((pool.active_workers / pool.total_workers) * 100) : 0

  return (
    <SectionCard title="Agent Pool">
      <MetricRow label="Total Workers" value={pool.total_workers.toString()} />
      <MetricRow label="Active Workers" value={pool.active_workers.toString()} />
      <MetricRow label="Idle Workers" value={pool.idle_workers.toString()} />
      <MetricRow label="Utilization" value={`${utilization}%`} />
      <MetricRow label="Queue Depth" value={pool.queue_depth.toString()} />
      <MetricRow label="Tasks Completed" value={pool.tasks_completed.toLocaleString()} />
      <MetricRow label="Tasks Failed" value={pool.tasks_failed.toLocaleString()} />
      <MetricRow label="Avg Task Duration" value={formatMs(pool.avg_task_duration_ms)} />
      <MetricRow label="P95 Task Duration" value={formatMs(pool.p95_task_duration_ms)} />
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
      <MetricRow label="Claimed Issues" value={dispatcher.claimed_count.toString()} />
      <MetricRow label="Total Routed" value={dispatcher.total_routed.toLocaleString()} />
      <MetricRow label="Total Requeued" value={dispatcher.total_requeued.toLocaleString()} />
      <MetricRow
        label="Events Processed"
        value={dispatcher.total_events_processed.toLocaleString()}
      />
      <MetricRow label="Avg Poll Latency" value={formatMs(dispatcher.avg_poll_latency_ms)} />
      <MetricRow label="Last Poll" value={lastPoll} />
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
              <div key={e.id} className="rounded bg-gray-50 px-3 py-2 text-xs">
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

export function Metrics() {
  const metrics = useQuery({
    queryKey: ['metrics'],
    queryFn: api.getMetrics,
    refetchInterval: 10_000,
  })

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h2 className="text-2xl font-bold text-gray-900">Metrics</h2>
        <span className="text-xs text-gray-400">Auto-refreshes every 10s</span>
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
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
            <PoolSection pool={metrics.data.pool} />
            <DispatcherSection dispatcher={metrics.data.dispatcher} />
            <SystemSection system={metrics.data.system} />
          </div>
          <ScalingSection
            events={metrics.data.scaling_events}
            config={metrics.data.scaling_config}
          />
        </div>
      )}
    </div>
  )
}
