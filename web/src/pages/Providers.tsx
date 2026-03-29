import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import type { AgentInfo, ActiveWorker, AgentSession, ProviderHealthEntry } from '../lib/api'

function typeBadgeColor(type: string): string {
  switch (type) {
    case 'claude':
    case 'claude-cli':
      return 'bg-orange-100 dark:bg-orange-900/30 text-orange-800 dark:text-orange-300'
    case 'ollama':
      return 'bg-purple-100 dark:bg-purple-900/30 text-purple-800 dark:text-purple-300'
    case 'openai':
      return 'bg-green-100 dark:bg-green-900/30 text-green-800 dark:text-green-300'
    default:
      return 'bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300'
  }
}

function statusColor(status: string): string {
  switch (status) {
    case 'completed':
      return 'text-green-600 dark:text-green-400'
    case 'failed':
      return 'text-red-600 dark:text-red-400'
    case 'active':
      return 'text-blue-600 dark:text-blue-400'
    case 'pending':
      return 'text-yellow-600 dark:text-yellow-400'
    default:
      return 'text-gray-500 dark:text-gray-400'
  }
}

function formatDuration(secs: number): string {
  if (secs < 60) return `${secs.toFixed(1)}s`
  const mins = Math.floor(secs / 60)
  const remSecs = Math.round(secs % 60)
  if (mins < 60) return `${mins}m ${remSecs}s`
  const hrs = Math.floor(mins / 60)
  const remMins = mins % 60
  return `${hrs}h ${remMins}m`
}

function formatElapsed(s: number): string {
  const m = Math.floor(s / 60)
  const sec = Math.floor(s % 60)
  return m > 0 ? `${m}m ${sec}s` : `${sec}s`
}

function formatLastSuccess(value: string | undefined): string {
  if (value == null || value === '') return '—'
  if (value === 'never') return 'Never'
  const d = new Date(value)
  if (isNaN(d.getTime())) return value
  return d.toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function Sparkline({ data, color = '#3b82f6', height = 28 }: { data: number[]; color?: string; height?: number }) {
  if (data.length < 2) return null
  const vw = 200
  const vh = 40
  const max = Math.max(...data)
  const min = Math.min(...data)
  const range = max - min || 1
  const points = data
    .map((v, i) => {
      const x = (i / (data.length - 1)) * vw
      const y = vh - ((v - min) / range) * (vh - 4) - 2
      return `${x},${y}`
    })
    .join(' ')

  return (
    <svg viewBox={`0 0 ${vw} ${vh}`} height={height} className="w-full min-w-0" preserveAspectRatio="none">
      <polyline fill="none" stroke={color} strokeWidth="2" points={points} />
    </svg>
  )
}

function RecentTaskRow({ session, activeWorker }: { session: AgentSession; activeWorker?: ActiveWorker }) {
  return (
    <div className="flex items-center gap-2 py-1.5 border-b dark:border-gray-700 last:border-b-0 text-xs">
      <span className={`font-medium ${statusColor(session.status)}`}>
        {session.status}
      </span>
      <a
        href={`https://github.com/HerbHall/samverk/issues/${session.issue_number}`}
        target="_blank"
        rel="noopener noreferrer"
        className="font-mono text-blue-600 dark:text-blue-400 hover:underline"
      >
        #{session.issue_number}
      </a>
      {activeWorker != null && (
        <span className="font-mono text-blue-500 dark:text-blue-400">
          {formatElapsed(activeWorker.elapsed_s)}
        </span>
      )}
      {activeWorker != null && activeWorker.model !== '' && (
        <span className="truncate text-gray-500 dark:text-gray-400 max-w-32" title={activeWorker.model}>
          {activeWorker.model}
        </span>
      )}
      <span className="flex-1" />
      {session.duration_secs != null && activeWorker == null && (
        <span className="font-mono text-gray-500 dark:text-gray-400">
          {formatDuration(session.duration_secs)}
        </span>
      )}
      <span className="text-gray-400 dark:text-gray-500">
        {new Date(session.started_at).toLocaleDateString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}
      </span>
    </div>
  )
}

interface MergedProvider {
  name: string
  healthy: boolean
  type: string
  model: string
  accountUrl?: string
  routingChains: string[]
  stats: AgentInfo['stats'] | null
  recentSessions: AgentSession[]
  throughput: AgentInfo['throughput']
  lastSuccessAt?: string
  modelLoaded?: boolean
  vramFreeBytes?: number
  vramTotalBytes?: number
  error?: string
}

function mergeProviderData(
  healthData: ProviderHealthEntry[] | undefined,
  agentData: AgentInfo[] | undefined,
): MergedProvider[] {
  const agentByName = new Map<string, AgentInfo>()
  for (const a of agentData ?? []) {
    agentByName.set(a.name, a)
  }

  const seen = new Set<string>()
  const result: MergedProvider[] = []

  for (const h of healthData ?? []) {
    seen.add(h.name)
    const agent = agentByName.get(h.name)
    result.push({
      name: h.name,
      healthy: h.healthy,
      type: agent?.type ?? '',
      model: agent?.model ?? '',
      accountUrl: agent?.account_url,
      routingChains: agent?.routing_chains ?? [],
      stats: agent?.stats ?? null,
      recentSessions: agent?.recent_sessions ?? [],
      throughput: agent?.throughput ?? [],
      lastSuccessAt: h.last_success_at,
      modelLoaded: h.model_loaded,
      vramFreeBytes: h.vram_free_bytes,
      vramTotalBytes: h.vram_total_bytes,
      error: h.error,
    })
  }

  // Add any agents not present in health data
  for (const a of agentData ?? []) {
    if (!seen.has(a.name)) {
      result.push({
        name: a.name,
        healthy: a.healthy,
        type: a.type,
        model: a.model,
        accountUrl: a.account_url,
        routingChains: a.routing_chains,
        stats: a.stats,
        recentSessions: a.recent_sessions,
        throughput: a.throughput,
      })
    }
  }

  return result
}

function ProviderCard({ provider, activeByIssue }: { provider: MergedProvider; activeByIssue: Map<number, ActiveWorker> }) {
  const [expanded, setExpanded] = useState(false)
  const throughputValues = provider.throughput.map((t) => t.count)
  const totalThroughput = throughputValues.reduce((a, b) => a + b, 0)

  return (
    <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 p-5">
      {/* Header: name, type badge, health */}
      <div className="flex items-center justify-between mb-1">
        <div className="flex items-center gap-2">
          <span
            className={`inline-block h-2.5 w-2.5 rounded-full ${provider.healthy ? 'bg-green-500' : 'bg-red-500'}`}
          />
          <h3 className="text-base font-semibold text-gray-900 dark:text-gray-100">{provider.name}</h3>
        </div>
        {provider.type !== '' && (
          <span
            className={`inline-block rounded-full px-2.5 py-0.5 text-xs font-medium ${typeBadgeColor(provider.type)}`}
          >
            {provider.type}
          </span>
        )}
      </div>

      {/* Model name */}
      {provider.model !== '' && (
        <p className="mb-3 font-mono text-sm text-gray-600 dark:text-gray-400">{provider.model}</p>
      )}

      {/* Provider health details: last success, VRAM, model loaded */}
      <div className="space-y-1 text-sm mb-3">
        {provider.lastSuccessAt != null && (
          <div className="flex items-center justify-between">
            <span className="text-gray-500 dark:text-gray-400">Last success</span>
            <span className="font-mono text-gray-700 dark:text-gray-300 text-xs">
              {formatLastSuccess(provider.lastSuccessAt)}
            </span>
          </div>
        )}

        {provider.modelLoaded != null && (
          <div className="flex items-center justify-between">
            <span className="text-gray-500 dark:text-gray-400">Model loaded</span>
            <span className={`text-xs font-medium ${provider.modelLoaded ? 'text-green-600 dark:text-green-400' : 'text-gray-500 dark:text-gray-400'}`}>
              {provider.modelLoaded ? 'Yes' : 'No'}
            </span>
          </div>
        )}

        {provider.vramTotalBytes != null && provider.vramTotalBytes > 0 && (
          <div className="flex items-center justify-between">
            <span className="text-gray-500 dark:text-gray-400">VRAM free</span>
            <span className="font-mono text-xs text-gray-700 dark:text-gray-300">
              {Math.round((provider.vramFreeBytes ?? 0) / 1024 / 1024 / 1024 * 10) / 10} GB
              {' / '}
              {Math.round(provider.vramTotalBytes / 1024 / 1024 / 1024 * 10) / 10} GB
            </span>
          </div>
        )}
      </div>

      {/* Routing chains */}
      {provider.routingChains.length > 0 && (
        <div className="mb-3 flex flex-wrap gap-1.5">
          {provider.routingChains.map((chain) => (
            <span
              key={chain}
              className="inline-block rounded-full bg-blue-50 dark:bg-blue-900/30 px-2.5 py-0.5 text-xs font-medium text-blue-700 dark:text-blue-300"
            >
              {chain}
            </span>
          ))}
        </div>
      )}

      {/* Stats grid */}
      {provider.stats != null && (
        <>
          <div className="grid grid-cols-3 gap-3 mb-3">
            <div className="rounded-md bg-gray-50 dark:bg-gray-700/50 p-2.5 text-center">
              <p className="text-lg font-bold text-gray-900 dark:text-gray-100">{provider.stats.total_sessions}</p>
              <p className="text-xs text-gray-500 dark:text-gray-400">Total Tasks</p>
            </div>
            <div className="rounded-md bg-gray-50 dark:bg-gray-700/50 p-2.5 text-center">
              <p className={`text-lg font-bold ${provider.stats.success_rate >= 80 ? 'text-green-600 dark:text-green-400' : provider.stats.success_rate >= 50 ? 'text-yellow-600 dark:text-yellow-400' : 'text-red-600 dark:text-red-400'}`}>
                {provider.stats.success_rate > 0 ? `${provider.stats.success_rate}%` : '--'}
              </p>
              <p className="text-xs text-gray-500 dark:text-gray-400">Success Rate</p>
            </div>
            <div className="rounded-md bg-gray-50 dark:bg-gray-700/50 p-2.5 text-center">
              <p className="text-lg font-bold text-gray-900 dark:text-gray-100">
                {provider.stats.avg_duration_secs > 0 ? formatDuration(provider.stats.avg_duration_secs) : '--'}
              </p>
              <p className="text-xs text-gray-500 dark:text-gray-400">Avg Duration</p>
            </div>
          </div>

          {/* Breakdown row */}
          <div className="flex items-center gap-4 mb-3 text-xs text-gray-500 dark:text-gray-400">
            <span>
              <span className="font-medium text-green-600 dark:text-green-400">{provider.stats.completed}</span> completed
            </span>
            <span>
              <span className="font-medium text-red-600 dark:text-red-400">{provider.stats.failed}</span> failed
            </span>
            {provider.stats.active > 0 && (
              <span>
                <span className="font-medium text-blue-600 dark:text-blue-400">{provider.stats.active}</span> active
              </span>
            )}
          </div>
        </>
      )}

      {/* Throughput sparkline */}
      {throughputValues.length >= 2 && (
        <div className="mb-3">
          <div className="flex items-center justify-between mb-1">
            <span className="text-xs text-gray-500 dark:text-gray-400">Throughput (24h)</span>
            <span className="text-xs font-mono text-gray-600 dark:text-gray-300">{totalThroughput} tasks</span>
          </div>
          <Sparkline data={throughputValues} color={provider.healthy ? '#22c55e' : '#ef4444'} />
        </div>
      )}

      {/* Error message */}
      {provider.error != null && provider.error !== '' && (
        <p className="mb-3 text-xs text-red-600 dark:text-red-400 break-words">{provider.error}</p>
      )}

      {/* Account link */}
      {provider.accountUrl != null && provider.accountUrl !== '' && (
        <div className="mb-3">
          <a
            href={provider.accountUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1 rounded border border-gray-300 dark:border-gray-600 px-3 py-1.5 text-xs font-medium text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700"
          >
            Manage Account
            <svg className="h-3 w-3" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
            </svg>
          </a>
        </div>
      )}

      {/* Recent tasks (collapsible) */}
      {provider.recentSessions.length > 0 && (
        <div>
          <button
            onClick={() => setExpanded(!expanded)}
            className="flex w-full items-center justify-between rounded px-2 py-1.5 text-xs font-medium text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700/50"
          >
            <span>Recent Tasks ({provider.recentSessions.length})</span>
            <svg
              className={`h-3.5 w-3.5 transition-transform ${expanded ? 'rotate-180' : ''}`}
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
              strokeWidth={2}
            >
              <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
            </svg>
          </button>
          {expanded && (
            <div className="mt-1 px-1">
              {provider.recentSessions.map((s) => (
                <RecentTaskRow key={s.id} session={s} activeWorker={activeByIssue.get(s.issue_number)} />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function RoutingChainsSection({ chains }: { chains: Record<string, string[]> }) {
  const entries = Object.entries(chains)
  if (entries.length === 0) return null

  return (
    <div className="mt-8">
      <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">Routing Chains</h3>
      <div className="space-y-3">
        {entries.map(([chain, providers]) => (
          <div
            key={chain}
            className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 px-5 py-4 flex items-center gap-4 flex-wrap"
          >
            <span className="text-sm font-medium text-gray-700 dark:text-gray-300 w-28 shrink-0">{chain}</span>
            <div className="flex items-center gap-2 flex-wrap">
              {providers.map((name, idx) => (
                <div key={`${name}-${idx}`} className="flex items-center gap-2">
                  {idx > 0 && (
                    <svg className="h-3 w-3 text-gray-400 dark:text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2.5}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
                    </svg>
                  )}
                  <span className="inline-block rounded-full bg-blue-50 dark:bg-blue-900/30 px-2.5 py-0.5 text-xs font-medium text-blue-700 dark:text-blue-300">
                    {name}
                  </span>
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

function EmptyState() {
  return (
    <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 p-10 text-center">
      <p className="text-sm text-gray-500 dark:text-gray-400">No providers configured</p>
      <p className="mt-1 text-xs text-gray-400 dark:text-gray-500">
        Add providers to .samverk/providers.yaml to see them here.
      </p>
    </div>
  )
}

export function Providers() {
  const health = useQuery({
    queryKey: ['providers'],
    queryFn: api.getProviderHealth,
    refetchInterval: 10_000,
  })

  const agents = useQuery({
    queryKey: ['agents'],
    queryFn: api.getAgents,
    refetchInterval: 10_000,
  })

  const activeWorkers = useQuery({
    queryKey: ['workers', 'active'],
    queryFn: api.getActiveWorkers,
    refetchInterval: 10_000,
  })

  const activeByIssue = useMemo(() => {
    const m = new Map<number, ActiveWorker>()
    for (const w of activeWorkers.data ?? []) {
      m.set(w.issue_number, w)
    }
    return m
  }, [activeWorkers.data])

  const merged = useMemo(
    () => mergeProviderData(health.data?.providers, agents.data?.agents),
    [health.data, agents.data],
  )

  const isLoading = health.isLoading && agents.isLoading
  const isError = health.isError || agents.isError
  const errorMessage = health.error?.message ?? agents.error?.message ?? 'Unknown error'

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h2 className="text-2xl font-bold text-gray-900 dark:text-gray-100">Providers</h2>
          {merged.length > 0 && (
            <span className="rounded-full bg-gray-100 dark:bg-gray-700 px-2.5 py-0.5 text-xs font-medium text-gray-600 dark:text-gray-300">
              {merged.length}
            </span>
          )}
        </div>
        <span className="text-xs text-gray-400 dark:text-gray-500">Auto-refreshes every 10s</span>
      </div>

      {isLoading && (
        <div className="flex items-center justify-center py-20">
          <p className="text-gray-500 dark:text-gray-400">Loading providers...</p>
        </div>
      )}

      {isError && (
        <div className="rounded-lg border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/30 p-4">
          <p className="font-medium text-red-800 dark:text-red-300">Failed to load provider data</p>
          <p className="mt-1 text-sm text-red-600 dark:text-red-400">{errorMessage}</p>
        </div>
      )}

      {!isLoading && !isError && merged.length === 0 && <EmptyState />}

      {merged.length > 0 && (
        <>
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            {merged.map((p) => (
              <ProviderCard key={p.name} provider={p} activeByIssue={activeByIssue} />
            ))}
          </div>

          {health.data?.routing_chains != null && Object.keys(health.data.routing_chains).length > 0 && (
            <RoutingChainsSection chains={health.data.routing_chains} />
          )}
        </>
      )}
    </div>
  )
}
