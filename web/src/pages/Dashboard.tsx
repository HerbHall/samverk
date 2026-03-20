import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, ActiveWorker } from '../lib/api'
import { useWSStore } from '../store/wsStore'

function StatusIndicator({ connected, label }: { connected: boolean; label: string }) {
  return (
    <div className="flex items-center gap-2">
      <span
        className={`inline-block h-2.5 w-2.5 rounded-full ${connected ? 'bg-green-500' : 'bg-red-500'}`}
      />
      <span className="text-sm text-gray-700 dark:text-gray-300">{label}</span>
      <span className={`text-xs ${connected ? 'text-green-600' : 'text-red-600'}`}>
        {connected ? 'Connected' : 'Disconnected'}
      </span>
    </div>
  )
}

function StatCard({ title, value, subtitle }: { title: string; value: string; subtitle?: string }) {
  return (
    <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 p-5">
      <p className="text-sm font-medium text-gray-500 dark:text-gray-400">{title}</p>
      <p className="mt-1 text-2xl font-semibold text-gray-900 dark:text-gray-100">{value}</p>
      {subtitle != null && <p className="mt-1 text-xs text-gray-400 dark:text-gray-500">{subtitle}</p>}
    </div>
  )
}

function formatElapsed(elapsedS: number): string {
  const m = Math.floor(elapsedS / 60)
  const s = Math.floor(elapsedS % 60)
  return m > 0 ? `${m}m ${s}s` : `${s}s`
}

function formatUSD(amount: number): string {
  return `$${amount.toFixed(4)}`
}

function formatTokens(count: number): string {
  if (count >= 1_000_000) {
    return `${(count / 1_000_000).toFixed(1)}M`
  }
  if (count >= 1_000) {
    return `${(count / 1_000).toFixed(1)}K`
  }
  return count.toString()
}

function WorkerCard({ w, issueTitles }: { w: ActiveWorker; issueTitles: Map<number, string> }) {
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
    <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 p-4 flex flex-col gap-2">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex items-center gap-2 mb-0.5">
            <a
              href={`https://github.com/HerbHall/samverk/issues/${w.issue_number}`}
              target="_blank"
              rel="noopener noreferrer"
              className="text-sm font-semibold text-blue-600 hover:underline dark:text-blue-400 shrink-0"
            >
              #{w.issue_number}
            </a>
            <span className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${agentColor}`}>
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

export function Dashboard() {
  const wsConnected = useWSStore((s) => s.connected)

  const status = useQuery({
    queryKey: ['status'],
    queryFn: api.getStatus,
    refetchInterval: wsConnected ? false : 30_000,
  })

  const sessions = useQuery({
    queryKey: ['sessions'],
    queryFn: api.listSessions,
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

  const issueTitles = useMemo(() => {
    const map = new Map<number, string>()
    issues.data?.forEach((i) => map.set(i.number, i.title))
    return map
  }, [issues.data])

  const isLoading = status.isLoading || costs.isLoading
  const hasError = status.isError || costs.isError

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-20">
        <p className="text-gray-500 dark:text-gray-400">Loading dashboard...</p>
      </div>
    )
  }

  if (hasError) {
    const errorMsg = status.error?.message || costs.error?.message || 'Unknown error'
    return (
      <div className="rounded-lg border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/30 p-4">
        <p className="font-medium text-red-800 dark:text-red-300">Failed to load dashboard</p>
        <p className="mt-1 text-sm text-red-600 dark:text-red-400">{errorMsg}</p>
      </div>
    )
  }

  const activeSessions = sessions.data?.filter((s) => s.status === 'running') ?? []
  const totalSessions = sessions.data?.length ?? 0

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h2 className="text-2xl font-bold text-gray-900 dark:text-gray-100">Dashboard</h2>
        <span className="inline-flex items-center gap-1 text-xs">
          <span className={`h-2 w-2 rounded-full ${wsConnected ? 'bg-green-500' : 'bg-gray-400'}`} />
          <span className="text-gray-500 dark:text-gray-400">
            {wsConnected ? 'Live' : 'Polling (30s)'}
          </span>
        </span>
      </div>

      <section className="mb-8">
        <h3 className="mb-3 text-sm font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
          System Status
        </h3>
        <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
          <div className="flex items-center gap-3 mb-4">
            <span
              className={`inline-block h-3 w-3 rounded-full ${status.data?.healthy ? 'bg-green-500' : 'bg-yellow-500'}`}
            />
            <span className="font-medium text-gray-900 dark:text-gray-100">
              {status.data?.healthy ? 'System Healthy' : 'System Degraded'}
            </span>
          </div>
          <div className="space-y-2">
            <StatusIndicator
              connected={status.data?.forge_connected ?? false}
              label="Git Forge"
            />
            <StatusIndicator
              connected={status.data?.database_connected ?? false}
              label="Database"
            />
          </div>
          <div className="mt-3 border-t dark:border-gray-700 pt-3">
            <p className="text-sm text-gray-500 dark:text-gray-400">
              MCP Tools:{' '}
              <span className="font-medium text-gray-700 dark:text-gray-300">
                {status.data?.tool_count ?? 0}
              </span>
            </p>
          </div>
        </div>
      </section>

      <section className="mb-8">
        <h3 className="mb-3 text-sm font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
          Activity
        </h3>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <StatCard
            title="Active Sessions"
            value={activeSessions.length.toString()}
            subtitle={`${totalSessions} total`}
          />
          <StatCard
            title="Total Cost"
            value={formatUSD(costs.data?.estimated_cost_usd ?? 0)}
            subtitle={`${formatTokens(costs.data?.tokens_used ?? 0)} tokens`}
          />
        </div>
      </section>

      <section>
        <h3 className="mb-3 text-sm font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
          Active Workers
        </h3>
        {activeWorkers.isLoading ? (
          <p className="text-sm text-gray-400 dark:text-gray-500">Loading...</p>
        ) : activeWorkers.data == null || activeWorkers.data.length === 0 ? (
          <p className="text-sm text-gray-400 dark:text-gray-500">No active workers</p>
        ) : (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {activeWorkers.data.map((w) => (
              <WorkerCard key={w.session_id} w={w} issueTitles={issueTitles} />
            ))}
          </div>
        )}
      </section>
    </div>
  )
}
