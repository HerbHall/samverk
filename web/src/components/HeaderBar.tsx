import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTheme } from '../hooks/useTheme'
import { api } from '../lib/api'
import { CapacityModal } from './CapacityModal'

interface HeaderBarProps {
  onChatToggle?: () => void
  chatOpen?: boolean
}

function CapacityIndicator() {
  const [open, setOpen] = useState(false)

  const hostMetrics = useQuery({
    queryKey: ['host-metrics'],
    queryFn: api.getHostMetrics,
    refetchInterval: 60_000,
    staleTime: 30_000,
  })

  const recs = hostMetrics.data?.recommendations ?? []
  const hasCritical = recs.some((r) => r.level === 'critical')
  const hasWarn = recs.some((r) => r.level === 'warn')

  const { dotColor, label } = hasCritical
    ? { dotColor: 'bg-red-500', label: 'Critical capacity issue' }
    : hasWarn
    ? { dotColor: 'bg-amber-500', label: 'Capacity warning' }
    : { dotColor: 'bg-green-500', label: 'All resources healthy' }

  const critCount = recs.filter((r) => r.level === 'critical').length
  const warnCount = recs.filter((r) => r.level === 'warn').length
  const tooltip = recs.length === 0
    ? 'Capacity data loading...'
    : hasCritical || hasWarn
    ? `${critCount > 0 ? critCount + ' critical' : ''}${critCount > 0 && warnCount > 0 ? ', ' : ''}${warnCount > 0 ? warnCount + ' warning' : ''} — click for details`
    : 'All resources healthy — click for details'

  return (
    <>
      <div className="relative group">
        <button
          onClick={() => setOpen(true)}
          className="flex items-center gap-1.5 rounded px-2 py-1 text-xs text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
          aria-label={label}
        >
          <span className={`h-2 w-2 rounded-full ${dotColor}`} />
          <span className="hidden sm:inline">Capacity</span>
        </button>
        <div className="pointer-events-none absolute right-0 top-full mt-1 z-50 hidden w-52 rounded bg-gray-900 dark:bg-gray-700 px-3 py-2 text-xs text-gray-100 shadow-lg group-hover:block">
          {tooltip}
        </div>
      </div>
      {open && <CapacityModal onClose={() => setOpen(false)} />}
    </>
  )
}

export function HeaderBar({ onChatToggle, chatOpen }: HeaderBarProps) {
  const { theme, toggle } = useTheme()
  const version = window.__SAMVERK_VERSION__ || 'dev'
  const commit = window.__SAMVERK_COMMIT__

  return (
    <header className="flex items-center justify-between border-b border-gray-200 bg-white px-6 py-2.5 dark:border-gray-700 dark:bg-gray-900">
      <div className="flex items-center gap-3">
        <span className="text-sm font-bold tracking-wider text-gray-900 dark:text-white">
          SAMVERK DASHBOARD
        </span>
        <span
          className="rounded bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600 dark:bg-gray-800 dark:text-gray-400"
          title={commit ? `Commit: ${commit}` : undefined}
        >
          {version}
        </span>
      </div>
      <div className="flex items-center gap-2">
        <CapacityIndicator />
        {onChatToggle != null && (
          <button
            onClick={onChatToggle}
            className={`rounded p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-gray-200 ${chatOpen === true ? 'bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400' : ''}`}
            title={chatOpen === true ? 'Close chat' : 'Chat with Claude'}
          >
            {'\u{1F4AC}'}
          </button>
        )}
        <button
          onClick={toggle}
          className="rounded p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-gray-200"
          title={theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
        >
          {theme === 'dark' ? '\u2600' : '\u263D'}
        </button>
      </div>
    </header>
  )
}
