import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import type { ActiveWorker, AgentInfo, AgentSession } from '../lib/api'

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

// Sparkline renders an inline SVG sparkline chart from numeric values.
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

function AgentCard({ agent, activeByIssue }: { agent: AgentInfo; activeByIssue: Map<number, ActiveWorker> }) {
  const [expanded, setExpanded] = useState(false)
  const throughputValues = agent.throughput.map((t) => t.count)
  const totalThroughput = throughputValues.reduce((a, b) => a + b, 0)

  return (
    <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 p-5">
      {/* Header: name, type badge, health */}
      <div className="flex items-center justify-between mb-1">
        <div className="flex items-center gap-2">
          <span
            className={`inline-block h-2.5 w-2.5 rounded-full ${agent.healthy ? 'bg-green-500' : 'bg-red-500'}`}
          />
          <h3 className="text-base font-semibold text-gray-900 dark:text-gray-100">{agent.name}</h3>
        </div>
        <span
          className={`inline-block rounded-full px-2.5 py-0.5 text-xs font-medium ${typeBadgeColor(agent.type)}`}
        >
          {agent.type}
        </span>
      </div>

      <p className="mb-3 font-mono text-sm text-gray-600 dark:text-gray-400">{agent.model}</p>

      {/* Routing chains */}
      {agent.routing_chains.length > 0 && (
        <div className="mb-3 flex flex-wrap gap-1.5">
          {agent.routing_chains.map((chain) => (
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
      <div className="grid grid-cols-3 gap-3 mb-3">
        <div className="rounded-md bg-gray-50 dark:bg-gray-700/50 p-2.5 text-center">
          <p className="text-lg font-bold text-gray-900 dark:text-gray-100">{agent.stats.total_sessions}</p>
          <p className="text-xs text-gray-500 dark:text-gray-400">Total Tasks</p>
        </div>
        <div className="rounded-md bg-gray-50 dark:bg-gray-700/50 p-2.5 text-center">
          <p className={`text-lg font-bold ${agent.stats.success_rate >= 80 ? 'text-green-600 dark:text-green-400' : agent.stats.success_rate >= 50 ? 'text-yellow-600 dark:text-yellow-400' : 'text-red-600 dark:text-red-400'}`}>
            {agent.stats.success_rate > 0 ? `${agent.stats.success_rate}%` : '--'}
          </p>
          <p className="text-xs text-gray-500 dark:text-gray-400">Success Rate</p>
        </div>
        <div className="rounded-md bg-gray-50 dark:bg-gray-700/50 p-2.5 text-center">
          <p className="text-lg font-bold text-gray-900 dark:text-gray-100">
            {agent.stats.avg_duration_secs > 0 ? formatDuration(agent.stats.avg_duration_secs) : '--'}
          </p>
          <p className="text-xs text-gray-500 dark:text-gray-400">Avg Duration</p>
        </div>
      </div>

      {/* Breakdown row */}
      <div className="flex items-center gap-4 mb-3 text-xs text-gray-500 dark:text-gray-400">
        <span>
          <span className="font-medium text-green-600 dark:text-green-400">{agent.stats.completed}</span> completed
        </span>
        <span>
          <span className="font-medium text-red-600 dark:text-red-400">{agent.stats.failed}</span> failed
        </span>
        {agent.stats.active > 0 && (
          <span>
            <span className="font-medium text-blue-600 dark:text-blue-400">{agent.stats.active}</span> active
          </span>
        )}
      </div>

      {/* Throughput sparkline */}
      {throughputValues.length >= 2 && (
        <div className="mb-3">
          <div className="flex items-center justify-between mb-1">
            <span className="text-xs text-gray-500 dark:text-gray-400">Throughput (24h)</span>
            <span className="text-xs font-mono text-gray-600 dark:text-gray-300">{totalThroughput} tasks</span>
          </div>
          <Sparkline data={throughputValues} color={agent.healthy ? '#22c55e' : '#ef4444'} />
        </div>
      )}

      {/* Account link */}
      {agent.account_url != null && agent.account_url !== '' && (
        <div className="mb-3">
          <a
            href={agent.account_url}
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
      {agent.recent_sessions.length > 0 && (
        <div>
          <button
            onClick={() => setExpanded(!expanded)}
            className="flex w-full items-center justify-between rounded px-2 py-1.5 text-xs font-medium text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700/50"
          >
            <span>Recent Tasks ({agent.recent_sessions.length})</span>
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
              {agent.recent_sessions.map((s) => (
                <RecentTaskRow key={s.id} session={s} activeWorker={activeByIssue.get(s.issue_number)} />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function EmptyState() {
  return (
    <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 p-10 text-center">
      <p className="text-sm text-gray-500 dark:text-gray-400">No agents configured</p>
      <p className="mt-1 text-xs text-gray-400 dark:text-gray-500">
        Add providers to .samverk/providers.yaml to see them here.
      </p>
    </div>
  )
}

export function Agents() {
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

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h2 className="text-2xl font-bold text-gray-900 dark:text-gray-100">Agents</h2>
        <span className="text-xs text-gray-400 dark:text-gray-500">Auto-refreshes every 10s</span>
      </div>

      {agents.isLoading && (
        <div className="flex items-center justify-center py-20">
          <p className="text-gray-500 dark:text-gray-400">Loading agents...</p>
        </div>
      )}

      {agents.isError && (
        <div className="rounded-lg border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/30 p-4">
          <p className="font-medium text-red-800 dark:text-red-300">Failed to load agent data</p>
          <p className="mt-1 text-sm text-red-600 dark:text-red-400">{agents.error?.message ?? 'Unknown error'}</p>
        </div>
      )}

      {agents.data != null && agents.data.agents.length === 0 && <EmptyState />}

      {agents.data != null && agents.data.agents.length > 0 && (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          {agents.data.agents.map((a) => (
            <AgentCard key={a.name} agent={a} activeByIssue={activeByIssue} />
          ))}
        </div>
      )}
    </div>
  )
}
