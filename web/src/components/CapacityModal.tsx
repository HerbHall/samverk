import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../lib/api'
import type { HostRecommendation, HostMetricsDTO } from '../lib/api'

function Sparkline({ data, color = '#4ade80' }: { data: number[]; color?: string }) {
  if (data.length < 2) return null
  const w = 120
  const h = 32
  const min = Math.min(...data)
  const max = Math.max(...data)
  const range = max - min || 1

  const pts = data.map((v, i) => ({
    x: (i / (data.length - 1)) * w,
    y: h - ((v - min) / range) * (h - 4) - 2,
  }))

  let d = `M ${pts[0].x} ${pts[0].y}`
  for (let i = 1; i < pts.length; i++) {
    const prev = pts[i - 1]
    const curr = pts[i]
    const cpx = (prev.x + curr.x) / 2
    d += ` C ${cpx} ${prev.y}, ${cpx} ${curr.y}, ${curr.x} ${curr.y}`
  }
  const fill = `${d} L ${pts[pts.length - 1].x} ${h} L ${pts[0].x} ${h} Z`

  return (
    <svg width={w} height={h} className="shrink-0 overflow-visible">
      <defs>
        <linearGradient id="cap-spark-fill" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={color} stopOpacity="0.3" />
          <stop offset="100%" stopColor={color} stopOpacity="0.02" />
        </linearGradient>
      </defs>
      <path d={fill} fill="url(#cap-spark-fill)" stroke="none" />
      <path d={d} fill="none" stroke={color} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function sparklineDataFor(resource: string, history: HostMetricsDTO[]): number[] {
  return history.map((h) => {
    switch (resource) {
      case 'cpu': return h.cpu_percent ?? 0
      case 'ram': return h.ram_percent
      case 'disk': return h.disk_percent
      case 'swap': return h.swap_percent
      default: return 0
    }
  })
}

function sparklineColor(level: string): string {
  if (level === 'critical') return '#f87171'
  if (level === 'warn') return '#fbbf24'
  return '#4ade80'
}

function levelIcon(level: string): string {
  if (level === 'critical') return '▲'
  if (level === 'warn') return '◆'
  return '●'
}

function levelTextColor(level: string): string {
  if (level === 'critical') return 'text-red-600 dark:text-red-400'
  if (level === 'warn') return 'text-amber-600 dark:text-amber-400'
  return 'text-green-600 dark:text-green-400'
}

const RESOURCE_LABEL: Record<string, string> = {
  cpu: 'CPU', ram: 'RAM', disk: 'Disk', swap: 'Swap',
}

interface ApplyState {
  status: 'idle' | 'loading' | 'success' | 'error'
  issueNumber?: number
  issueURL?: string
  error?: string
}

function RecommendationCard({
  rec,
  history,
}: {
  rec: HostRecommendation
  history: HostMetricsDTO[]
}) {
  const queryClient = useQueryClient()
  const [applyState, setApplyState] = useState<ApplyState>({ status: 'idle' })

  const mutation = useMutation({
    mutationFn: () => api.applyCapacityRecommendation(rec.resource),
    onMutate: () => setApplyState({ status: 'loading' }),
    onSuccess: (data) => {
      setApplyState({ status: 'success', issueNumber: data.issue_number, issueURL: data.issue_url })
      void queryClient.invalidateQueries({ queryKey: ['host-metrics'] })
    },
    onError: (err: Error) => setApplyState({ status: 'error', error: err.message }),
  })

  const sparkData = sparklineDataFor(rec.resource, history)
  const color = sparklineColor(rec.level)
  const canApply = rec.action_type === 'proxmox_config' && rec.action != null

  return (
    <div className="border-b dark:border-gray-700 last:border-b-0 px-5 py-4">
      <div className="flex items-start justify-between gap-3 mb-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 mb-1">
            <span className={`text-xs font-semibold ${levelTextColor(rec.level)}`}>
              {levelIcon(rec.level)} {RESOURCE_LABEL[rec.resource] ?? rec.resource}
            </span>
          </div>
          <p className="text-sm font-medium text-gray-900 dark:text-gray-100 leading-snug">{rec.title}</p>
          <p className="mt-1 text-xs text-gray-500 dark:text-gray-400 leading-relaxed">{rec.detail}</p>
        </div>
        {sparkData.length >= 2 && (
          <div className="shrink-0 pt-1">
            <Sparkline data={sparkData} color={color} />
          </div>
        )}
      </div>

      {rec.stats != null && rec.stats.sample_count > 0 && (
        <div className="grid grid-cols-3 gap-x-4 gap-y-1 mb-3 bg-gray-50 dark:bg-gray-900 rounded px-3 py-2 text-xs">
          <div><span className="text-gray-400 dark:text-gray-500">avg</span> <span className="font-medium text-gray-700 dark:text-gray-300">{rec.stats.avg.toFixed(1)}%</span></div>
          <div><span className="text-gray-400 dark:text-gray-500">p95</span> <span className="font-medium text-gray-700 dark:text-gray-300">{rec.stats.p95.toFixed(1)}%</span></div>
          <div><span className="text-gray-400 dark:text-gray-500">peak</span> <span className="font-medium text-gray-700 dark:text-gray-300">{rec.stats.peak.toFixed(1)}%</span></div>
          {rec.stats.days_to_crit > 0 && (
            <div className="col-span-3 mt-0.5">
              <span className="text-gray-400 dark:text-gray-500">critical in</span>{' '}
              <span className="font-medium text-red-600 dark:text-red-400">~{Math.round(rec.stats.days_to_crit)} days</span>
            </div>
          )}
        </div>
      )}

      {canApply && rec.action != null && (
        <div className="rounded border dark:border-gray-700 bg-gray-50 dark:bg-gray-900 px-3 py-2 text-xs space-y-1 mb-2">
          <p className="font-medium text-gray-700 dark:text-gray-300">{rec.action.description}</p>
          <p className="text-gray-500 dark:text-gray-400">{rec.action.cost_note}</p>
          <code className="block font-mono text-gray-600 dark:text-gray-400 text-xs mt-1">
            {'ssh ' + rec.action.ssh_target + " '" + rec.action.command + "'"}
          </code>
        </div>
      )}

      {rec.action_type === 'hardware_upgrade' && (
        <div className="rounded bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-700 px-3 py-2 text-xs text-amber-700 dark:text-amber-300 mb-2">
          Requires hardware evaluation — cannot be applied automatically
        </div>
      )}

      {canApply && (
        <div>
          {applyState.status === 'idle' && (
            <button
              onClick={() => mutation.mutate()}
              className="rounded bg-blue-600 hover:bg-blue-700 text-white text-xs font-medium px-3 py-1.5 transition-colors"
            >
              Apply — create draft issue for review
            </button>
          )}
          {applyState.status === 'loading' && (
            <span className="text-xs text-gray-400 dark:text-gray-500">Creating issue...</span>
          )}
          {applyState.status === 'success' && (
            <span className="text-xs text-green-600 dark:text-green-400">
              Dispatched —{' '}
              <a
                href={applyState.issueURL}
                target="_blank"
                rel="noopener noreferrer"
                className="underline"
              >
                issue #{applyState.issueNumber}
              </a>
            </span>
          )}
          {applyState.status === 'error' && (
            <span className="text-xs text-red-600 dark:text-red-400">
              Failed: {applyState.error}{' '}
              <button
                onClick={() => setApplyState({ status: 'idle' })}
                className="underline"
              >
                retry
              </button>
            </span>
          )}
        </div>
      )}
    </div>
  )
}

export function CapacityModal({ onClose }: { onClose: () => void }) {
  const hostMetrics = useQuery({
    queryKey: ['host-metrics'],
    queryFn: api.getHostMetrics,
    refetchInterval: 60_000,
    staleTime: 30_000,
  })

  const recs = hostMetrics.data?.recommendations ?? []
  const history = hostMetrics.data?.history ?? []

  const critical = recs.filter((r) => r.level === 'critical')
  const warn = recs.filter((r) => r.level === 'warn')
  const info = recs.filter((r) => r.level === 'info')
  const ordered = [...critical, ...warn, ...info]

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 z-40 bg-black/40 backdrop-blur-sm"
        onClick={onClose}
      />
      {/* Panel */}
      <div className="fixed right-0 top-0 bottom-0 z-50 w-full max-w-md bg-white dark:bg-gray-800 shadow-2xl flex flex-col">
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b dark:border-gray-700 bg-gray-50 dark:bg-gray-900">
          <div>
            <h2 className="text-sm font-semibold text-gray-900 dark:text-gray-100">Capacity Advisor</h2>
            <p className="text-xs text-gray-500 dark:text-gray-400">24h analysis window</p>
          </div>
          <button
            onClick={onClose}
            className="rounded p-1 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
            aria-label="Close"
          >
            <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
            </svg>
          </button>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto">
          {hostMetrics.isLoading && (
            <div className="flex items-center justify-center py-10 text-sm text-gray-400 dark:text-gray-500">
              Loading...
            </div>
          )}
          {hostMetrics.isError && (
            <div className="px-5 py-4 text-sm text-red-600 dark:text-red-400">
              Failed to load host metrics
            </div>
          )}
          {!hostMetrics.isLoading && ordered.length === 0 && (
            <div className="px-5 py-10 text-center">
              <p className="text-2xl mb-2">&#x2713;</p>
              <p className="text-sm text-gray-500 dark:text-gray-400">All resources healthy</p>
            </div>
          )}
          {ordered.map((rec) => (
            <RecommendationCard key={rec.resource} rec={rec} history={history} />
          ))}
        </div>
      </div>
    </>
  )
}
