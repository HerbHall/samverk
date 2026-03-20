import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import type { ActiveWorker, LogEntry } from '../lib/api'

function levelColor(level: string): string {
  switch (level.toLowerCase()) {
    case 'error':
    case 'fatal':
      return 'text-red-600 dark:text-red-400'
    case 'warn':
    case 'warning':
      return 'text-amber-600 dark:text-amber-400'
    case 'debug':
      return 'text-gray-400 dark:text-gray-500'
    default:
      return 'text-gray-700 dark:text-gray-300'
  }
}

function formatElapsed(elapsedS: number): string {
  const m = Math.floor(elapsedS / 60)
  const s = Math.floor(elapsedS % 60)
  return m > 0 ? `${m}m ${s}s` : `${s}s`
}

function LogRow({ entry }: { entry: LogEntry }) {
  const ts = new Date(entry.ts).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })

  return (
    <div className="flex gap-2 py-0.5 text-xs font-mono leading-relaxed">
      <span className="shrink-0 text-gray-400 dark:text-gray-500 tabular-nums">{ts}</span>
      <span className={`shrink-0 w-10 uppercase font-semibold ${levelColor(entry.level)}`}>
        {entry.level.slice(0, 4)}
      </span>
      {entry.component != null && entry.component !== '' && (
        <span className="shrink-0 text-blue-500 dark:text-blue-400 max-w-24 truncate" title={entry.component}>
          [{entry.component}]
        </span>
      )}
      <span className="text-gray-800 dark:text-gray-200 break-all">{entry.msg}</span>
    </div>
  )
}

interface WorkerDetailPanelProps {
  worker: ActiveWorker | null
  onClose: () => void
}

export function WorkerDetailPanel({ worker, onClose }: WorkerDetailPanelProps) {
  // Escape key handler
  useEffect(() => {
    if (worker == null) return
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [worker, onClose])

  const sessionLog = useQuery({
    queryKey: ['session-log', worker?.session_id],
    queryFn: () => api.getSessionLog(worker!.session_id),
    enabled: worker != null,
    refetchInterval: worker != null ? 10_000 : false,
  })

  if (worker == null) return null

  const entries: LogEntry[] = sessionLog.data?.entries ?? []
  const startedAt = new Date(worker.started_at)

  const agentColor =
    worker.agent_type === 'code-gen'
      ? 'bg-blue-100 dark:bg-blue-900/40 text-blue-700 dark:text-blue-300'
      : worker.agent_type === 'docs'
        ? 'bg-purple-100 dark:bg-purple-900/40 text-purple-700 dark:text-purple-300'
        : 'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-400'

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 z-40 bg-black/40 backdrop-blur-sm"
        onClick={onClose}
      />
      {/* Slide-over panel */}
      <div className="fixed right-0 top-0 bottom-0 z-50 w-full max-w-lg bg-white dark:bg-gray-800 shadow-2xl flex flex-col">
        {/* Header */}
        <div className="flex items-start justify-between gap-3 px-5 py-4 border-b dark:border-gray-700 bg-gray-50 dark:bg-gray-900">
          <div className="min-w-0">
            <div className="flex items-center gap-2 mb-1">
              <span className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${agentColor}`}>
                {worker.agent_type}
              </span>
              <span className="inline-block h-2 w-2 rounded-full bg-green-400 animate-pulse" title="Running" />
            </div>
            <h2 className="text-sm font-semibold text-gray-900 dark:text-gray-100">
              Session #{worker.issue_number}
            </h2>
            <p className="text-xs text-gray-500 dark:text-gray-400 font-mono truncate mt-0.5" title={worker.session_id}>
              {worker.session_id}
            </p>
          </div>
          <button
            onClick={onClose}
            className="shrink-0 rounded p-1 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
            aria-label="Close"
          >
            <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
            </svg>
          </button>
        </div>

        {/* Metadata grid */}
        <div className="grid grid-cols-2 gap-x-6 gap-y-2 px-5 py-3 border-b dark:border-gray-700 text-xs">
          <div className="flex justify-between">
            <span className="text-gray-500 dark:text-gray-400">Provider</span>
            <span className="font-medium text-gray-700 dark:text-gray-300 truncate max-w-[55%] text-right">
              {worker.provider}
            </span>
          </div>
          <div className="flex justify-between">
            <span className="text-gray-500 dark:text-gray-400">Model</span>
            <span className="font-mono text-gray-600 dark:text-gray-400 truncate max-w-[55%] text-right" title={worker.model}>
              {worker.model}
            </span>
          </div>
          <div className="flex justify-between">
            <span className="text-gray-500 dark:text-gray-400">Started</span>
            <span className="text-gray-600 dark:text-gray-400">
              {startedAt.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
            </span>
          </div>
          <div className="flex justify-between">
            <span className="text-gray-500 dark:text-gray-400">Elapsed</span>
            <span className="font-medium text-gray-700 dark:text-gray-300">
              {formatElapsed(worker.elapsed_s)}
            </span>
          </div>
        </div>

        {/* Log section */}
        <div className="flex items-center justify-between px-5 py-2 border-b dark:border-gray-700 bg-gray-50 dark:bg-gray-900">
          <span className="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
            Session Log
          </span>
          <span className="text-xs text-gray-400 dark:text-gray-500">
            {sessionLog.isLoading ? 'Loading...' : `${entries.length} entries`}
          </span>
        </div>

        <div className="flex-1 overflow-y-auto px-4 py-3">
          {sessionLog.isLoading && (
            <p className="text-xs text-gray-400 dark:text-gray-500 py-4 text-center">Loading log...</p>
          )}
          {sessionLog.isError && (
            <div className="rounded border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/20 px-3 py-2">
              <p className="text-xs text-red-600 dark:text-red-400">
                Failed to load log: {sessionLog.error instanceof Error ? sessionLog.error.message : 'Unknown error'}
              </p>
            </div>
          )}
          {!sessionLog.isLoading && entries.length === 0 && !sessionLog.isError && (
            <p className="text-xs text-gray-400 dark:text-gray-500 py-4 text-center">No log entries yet</p>
          )}
          {entries.map((entry) => (
            <LogRow key={entry.id} entry={entry} />
          ))}
        </div>
      </div>
    </>
  )
}
