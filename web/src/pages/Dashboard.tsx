import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { AreaChart, Area, ResponsiveContainer, Tooltip } from 'recharts'
import type { TooltipContentProps } from 'recharts'
import { Link } from 'react-router'
import { api, ActiveWorker, HistoryEntry, Issue, ProviderHealthEntry } from '../lib/api'
import { useWSStore } from '../store/wsStore'
import { WorkerDetailPanel } from '../components/WorkerDetailPanel'

// ── Helpers ──────────────────────────────────────────────────────────────────

function formatElapsed(elapsedS: number): string {
  const m = Math.floor(elapsedS / 60)
  const s = Math.floor(elapsedS % 60)
  return m > 0 ? `${m}m ${s}s` : `${s}s`
}

function formatUSD(amount: number): string {
  return `$${amount.toFixed(4)}`
}

function formatTokens(count: number): string {
  if (count >= 1_000_000) return `${(count / 1_000_000).toFixed(1)}M`
  if (count >= 1_000) return `${(count / 1_000).toFixed(1)}K`
  return count.toString()
}

// ── Row 1: System Health Banner ───────────────────────────────────────────────

type BannerLevel = 'green' | 'yellow' | 'red'

interface BannerInfo {
  level: BannerLevel
  reasons: string[]
}

function computeBanner(
  statusHealthy: boolean,
  forgeConnected: boolean,
  dbConnected: boolean,
  providers: ProviderHealthEntry[] | undefined,
  providersError: boolean,
): BannerInfo {
  const reasons: string[] = []

  if (!forgeConnected) reasons.push('Git forge disconnected')
  if (!dbConnected) reasons.push('Database disconnected')
  if (!statusHealthy) reasons.push('System health check failed')

  if (!providersError && providers != null) {
    const unhealthy = providers.filter((p) => !p.healthy).length
    if (unhealthy > 0) reasons.push(`${unhealthy} provider${unhealthy === 1 ? '' : 's'} offline`)
  }

  let level: BannerLevel
  if (!forgeConnected || !dbConnected || !statusHealthy) {
    level = 'red'
  } else if (reasons.length > 0) {
    level = 'yellow'
  } else {
    level = 'green'
  }

  return { level, reasons }
}

function HealthBanner({
  banner,
  providersLoading,
}: {
  banner: BannerInfo
  providersLoading: boolean
}) {
  const levelStyles: Record<BannerLevel, string> = {
    green: 'bg-green-50 border-green-200 dark:bg-green-900/20 dark:border-green-800',
    yellow: 'bg-yellow-50 border-yellow-200 dark:bg-yellow-900/20 dark:border-yellow-800',
    red: 'bg-red-50 border-red-200 dark:bg-red-900/20 dark:border-red-800',
  }
  const dotStyles: Record<BannerLevel, string> = {
    green: 'bg-green-500',
    yellow: 'bg-yellow-500',
    red: 'bg-red-500',
  }
  const textStyles: Record<BannerLevel, string> = {
    green: 'text-green-800 dark:text-green-300',
    yellow: 'text-yellow-800 dark:text-yellow-300',
    red: 'text-red-800 dark:text-red-300',
  }

  const { level, reasons } = banner
  const label =
    level === 'green'
      ? 'All systems nominal'
      : level === 'yellow'
        ? `Degraded — ${reasons.join(', ')}`
        : `Critical — ${reasons.join(', ')}`

  return (
    <div className={`rounded-lg border px-4 py-3 flex items-center gap-3 ${levelStyles[level]}`}>
      <span className={`inline-block h-3 w-3 rounded-full shrink-0 ${dotStyles[level]}`} />
      <span className={`text-sm font-medium ${textStyles[level]}`}>{label}</span>
      {providersLoading && (
        <span className="ml-auto text-xs text-gray-400 dark:text-gray-500">
          Checking providers…
        </span>
      )}
    </div>
  )
}

// ── Row 2: Needs Attention ────────────────────────────────────────────────────

function AttentionBadge({
  label,
  count,
  color,
}: {
  label: string
  count: number
  color: string
}) {
  return (
    <div className={`flex items-center gap-2 rounded-lg border px-4 py-3 ${color}`}>
      <span className="text-xl font-bold">{count}</span>
      <span className="text-sm font-medium">{label}</span>
    </div>
  )
}

function NeedsAttentionRow({ issues }: { issues: Issue[] }) {
  const needsHuman = useMemo(
    () => issues.filter((i) => i.labels.includes('status:needs-human')),
    [issues],
  )
  const blocked = useMemo(
    () => issues.filter((i) => i.labels.includes('status:blocked')),
    [issues],
  )

  if (needsHuman.length === 0 && blocked.length === 0) return null

  return (
    <section className="mb-6">
      <h3 className="mb-3 text-sm font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
        Needs Attention
      </h3>
      <div className="flex flex-wrap gap-3">
        {needsHuman.length > 0 && (
          <Link to="/issues" className="no-underline">
            <AttentionBadge
              label="need human review"
              count={needsHuman.length}
              color="border-orange-200 bg-orange-50 text-orange-800 dark:border-orange-800 dark:bg-orange-900/20 dark:text-orange-300 hover:border-orange-400 dark:hover:border-orange-600 transition-colors cursor-pointer"
            />
          </Link>
        )}
        {blocked.length > 0 && (
          <Link to="/issues" className="no-underline">
            <AttentionBadge
              label="blocked"
              count={blocked.length}
              color="border-red-200 bg-red-50 text-red-800 dark:border-red-800 dark:bg-red-900/20 dark:text-red-300 hover:border-red-400 dark:hover:border-red-600 transition-colors cursor-pointer"
            />
          </Link>
        )}
      </div>
    </section>
  )
}

// ── Row 3: System Overview Stat Cards ─────────────────────────────────────────

function OverviewCard({
  title,
  value,
  subtitle,
  to,
}: {
  title: string
  value: string
  subtitle?: string
  to?: string
}) {
  const inner = (
    <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 p-5 hover:border-blue-300 dark:hover:border-blue-700 transition-colors h-full">
      <p className="text-sm font-medium text-gray-500 dark:text-gray-400">{title}</p>
      <p className="mt-1 text-2xl font-semibold text-gray-900 dark:text-gray-100">{value}</p>
      {subtitle != null && (
        <p className="mt-1 text-xs text-gray-400 dark:text-gray-500">{subtitle}</p>
      )}
    </div>
  )

  if (to != null) {
    return (
      <Link to={to} className="no-underline">
        {inner}
      </Link>
    )
  }
  return inner
}

// ── Row 4: Active Worker Card ─────────────────────────────────────────────────

function WorkerCard({
  w,
  issueTitles,
  onClick,
}: {
  w: ActiveWorker
  issueTitles: Map<number, string>
  onClick: () => void
}) {
  const issueTitle = issueTitles.get(w.issue_number)
  const startedAt = new Date(w.started_at)
  const elapsed = formatElapsed(w.elapsed_s)

  const agentColor =
    w.agent_type === 'code-gen'
      ? 'bg-blue-100 dark:bg-blue-900/40 text-blue-700 dark:text-blue-300'
      : w.agent_type === 'docs'
        ? 'bg-purple-100 dark:bg-purple-900/40 text-purple-700 dark:text-purple-300'
        : 'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-400'

  return (
    <div
      className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 p-4 flex flex-col gap-2 cursor-pointer hover:border-blue-400 dark:hover:border-blue-500 transition-colors"
      onClick={onClick}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') onClick()
      }}
      aria-label={`View session details for issue #${w.issue_number}`}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex items-center gap-2 mb-0.5">
            <span
              className="text-sm font-semibold text-blue-600 dark:text-blue-400 shrink-0"
            >
              IS#{w.issue_number}
            </span>
            <span
              className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${agentColor}`}
            >
              {w.agent_type}
            </span>
          </div>
          {issueTitle != null && (
            <p className="text-sm text-gray-900 dark:text-gray-100 leading-snug line-clamp-2">
              {issueTitle}
            </p>
          )}
        </div>
        <span
          className="shrink-0 inline-block h-2 w-2 rounded-full bg-green-400 animate-pulse mt-1"
          title="Running"
        />
      </div>

      <div className="border-t dark:border-gray-700 pt-2 space-y-1">
        <div className="flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
          <span>Provider</span>
          <span className="font-medium text-gray-700 dark:text-gray-300 truncate max-w-[60%] text-right">
            {w.provider}
          </span>
        </div>
        <div className="flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
          <span>Model</span>
          <span className="font-mono text-gray-600 dark:text-gray-400 truncate max-w-[60%] text-right">
            {w.model}
          </span>
        </div>
        <div className="flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
          <span>Started</span>
          <span className="text-gray-600 dark:text-gray-400">
            {startedAt.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
          </span>
        </div>
        <div className="flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
          <span>Elapsed</span>
          <span className="font-medium text-gray-700 dark:text-gray-300">{elapsed}</span>
        </div>
      </div>
    </div>
  )
}

function QueueDepthTooltip({ active, payload, label }: Partial<TooltipContentProps<number, string>>) {
  if (!active || payload == null || payload.length === 0) return null
  return (
    <div className="rounded border dark:border-gray-600 bg-white dark:bg-gray-800 px-2 py-1 text-xs shadow">
      <p className="text-gray-500 dark:text-gray-400">{label}</p>
      <p className="font-medium text-gray-900 dark:text-gray-100">{payload[0].value} queued</p>
    </div>
  )
}

function QueueDepthSparkline({
  entries,
  isLoading,
}: {
  entries: HistoryEntry[]
  isLoading: boolean
}) {
  const data = entries
    .filter((e) => e.pool != null)
    .map((e) => ({
      time: new Date(e.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
      queue_depth: e.pool!.queue_depth,
    }))

  return (
    <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
      {isLoading ? (
        <p className="text-sm text-gray-400 dark:text-gray-500">Loading...</p>
      ) : data.length === 0 ? (
        <p className="text-sm text-gray-400 dark:text-gray-500">No data</p>
      ) : (
        <ResponsiveContainer width="100%" height={120}>
          <AreaChart data={data} margin={{ top: 4, right: 4, bottom: 0, left: 0 }}>
            <defs>
              <linearGradient id="queueFill" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="#6366f1" stopOpacity={0.25} />
                <stop offset="95%" stopColor="#6366f1" stopOpacity={0.02} />
              </linearGradient>
            </defs>
            <Tooltip content={<QueueDepthTooltip />} />
            <Area
              type="monotone"
              dataKey="queue_depth"
              stroke="#6366f1"
              strokeWidth={1.5}
              fill="url(#queueFill)"
              dot={false}
              isAnimationActive={false}
            />
          </AreaChart>
        </ResponsiveContainer>
      )}
    </div>
  )
}

// ── Main Dashboard ────────────────────────────────────────────────────────────

export function Dashboard() {
  const wsConnected = useWSStore((s) => s.connected)
  const [selectedWorker, setSelectedWorker] = useState<ActiveWorker | null>(null)

  const status = useQuery({
    queryKey: ['status'],
    queryFn: api.getStatus,
    refetchInterval: wsConnected ? false : 30_000,
  })

  const costs = useQuery({
    queryKey: ['costs'],
    queryFn: api.getCosts,
    refetchInterval: wsConnected ? false : 30_000,
  })

  const activeWorkers = useQuery({
    queryKey: ['workers', 'active'],
    queryFn: api.getActiveWorkers,
    refetchInterval: 15_000,
  })

  const issues = useQuery({
    queryKey: ['issues', 'open'],
    queryFn: () => api.listIssues({ state: 'open' }),
    refetchInterval: 60_000,
    staleTime: 30_000,
  })

  const issueSummary = useQuery({
    queryKey: ['issue-summary'],
    queryFn: api.getIssueSummary,
    refetchInterval: 60_000,
  })

  // Provider health may return 503 -- handle gracefully (show unknown, don't crash)
  const providerHealth = useQuery({
    queryKey: ['providers', 'health'],
    queryFn: api.getProviderHealth,
    refetchInterval: 30_000,
    retry: 1,
  })

  const metricsHistory = useQuery({
    queryKey: ['metrics', 'history', '30m'],
    queryFn: () => api.getMetricsHistory('30m'),
    refetchInterval: 30_000,
  })

  const issueTitles = useMemo(() => {
    const map = new Map<number, string>()
    issues.data?.forEach((i) => map.set(i.number, i.title))
    return map
  }, [issues.data])

  const isLoading = status.isLoading || costs.isLoading

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-20">
        <p className="text-gray-500 dark:text-gray-400">Loading dashboard...</p>
      </div>
    )
  }

  if (status.isError) {
    return (
      <div className="rounded-lg border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/30 p-4">
        <p className="font-medium text-red-800 dark:text-red-300">Failed to load dashboard</p>
        <p className="mt-1 text-sm text-red-600 dark:text-red-400">
          {status.error?.message ?? 'Unknown error'}
        </p>
      </div>
    )
  }

  // Derived values
  const activeWorkerList = activeWorkers.data ?? []
  const openIssueCount = issueSummary.data?.aggregate.open ?? issues.data?.length ?? 0

  const healthyProviders =
    !providerHealth.isError && providerHealth.data != null
      ? providerHealth.data.providers.filter((p) => p.healthy).length
      : null
  const totalProviders =
    !providerHealth.isError && providerHealth.data != null ? providerHealth.data.count : null

  const banner = computeBanner(
    status.data?.healthy ?? false,
    status.data?.forge_connected ?? false,
    status.data?.database_connected ?? false,
    providerHealth.isError ? undefined : providerHealth.data?.providers,
    providerHealth.isError,
  )

  const providersValue =
    healthyProviders != null && totalProviders != null
      ? `${healthyProviders}/${totalProviders} healthy`
      : providerHealth.isError
        ? 'unknown'
        : 'checking…'

  const providersSubtitle = providerHealth.isError ? 'health check unavailable' : undefined

  return (
    <div>
      <WorkerDetailPanel worker={selectedWorker} onClose={() => setSelectedWorker(null)} />

      <div className="mb-6 flex items-center justify-between">
        <h2 className="text-2xl font-bold text-gray-900 dark:text-gray-100">Dashboard</h2>
        <span className="inline-flex items-center gap-1 text-xs">
          <span
            className={`h-2 w-2 rounded-full ${wsConnected ? 'bg-green-500' : 'bg-gray-400'}`}
          />
          <span className="text-gray-500 dark:text-gray-400">
            {wsConnected ? 'Live' : 'Polling (30s)'}
          </span>
        </span>
      </div>

      {/* Row 1: System Health Banner */}
      <div className="mb-6">
        <HealthBanner banner={banner} providersLoading={providerHealth.isLoading} />
      </div>

      {/* Row 2: Needs Attention (only shown when flagged issues exist) */}
      {issues.data != null && <NeedsAttentionRow issues={issues.data} />}

      {/* Row 3: System Overview */}
      <section className="mb-6">
        <h3 className="mb-3 text-sm font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
          System Overview
        </h3>
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          <OverviewCard
            title="Active Workers"
            value={activeWorkerList.length.toString()}
            subtitle="of pool capacity"
            to="/metrics"
          />
          <OverviewCard
            title="Total Open Issues"
            value={openIssueCount.toString()}
            subtitle={issues.isLoading ? 'loading…' : 'all open across all stages'}
            to="/issues"
          />
          <OverviewCard
            title="Cost Today"
            value={formatUSD(costs.data?.estimated_cost_usd ?? 0)}
            subtitle={`${formatTokens(costs.data?.tokens_used ?? 0)} tokens`}
          />
          <OverviewCard
            title="Providers"
            value={providersValue}
            subtitle={providersSubtitle}
            to="/providers"
          />
        </div>
      </section>

      {/* Row 4: Active Worker Cards */}
      <section className="mb-8">
        <h3 className="mb-3 text-sm font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
          Active Workers
        </h3>
        {activeWorkers.isLoading ? (
          <p className="text-sm text-gray-400 dark:text-gray-500">Loading...</p>
        ) : activeWorkerList.length === 0 ? (
          <p className="text-sm text-gray-400 dark:text-gray-500">No active workers</p>
        ) : (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {activeWorkerList.map((w) => (
              <WorkerCard
                key={w.session_id}
                w={w}
                issueTitles={issueTitles}
                onClick={() => setSelectedWorker(w)}
              />
            ))}
          </div>
        )}
      </section>

      <section>
        <h3 className="mb-3 text-sm font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
          Queue Depth (30m)
        </h3>
        <QueueDepthSparkline
          entries={metricsHistory.data?.entries ?? []}
          isLoading={metricsHistory.isLoading}
        />
      </section>
    </div>
  )
}
