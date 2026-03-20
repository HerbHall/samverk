import { useState, useEffect, useCallback, Fragment } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, LogEntry } from '../lib/api'
import { useWSStore } from '../store/wsStore'

function relativeTime(ts: string): string {
  const diff = Date.now() - new Date(ts).getTime()
  if (diff < 60_000) return `${Math.floor(diff / 1000)}s ago`
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}m ago`
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)}h ago`
  return `${Math.floor(diff / 86_400_000)}d ago`
}

function formatJSON(s: string): string {
  try {
    return JSON.stringify(JSON.parse(s), null, 2)
  } catch {
    return s
  }
}

function LevelBadge({ level }: { level: string }) {
  const colors: Record<string, string> = {
    debug: 'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300',
    info: 'bg-blue-100 dark:bg-blue-900/40 text-blue-700 dark:text-blue-300',
    warn: 'bg-amber-100 dark:bg-amber-900/40 text-amber-800 dark:text-amber-300',
    error: 'bg-red-100 dark:bg-red-900/40 text-red-800 dark:text-red-300',
  }
  return (
    <span
      className={`inline-block rounded px-1.5 py-0.5 text-xs font-medium ${colors[level] || 'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300'}`}
    >
      {level}
    </span>
  )
}

const PAGE_SIZE = 100
const LIVE_CAP = 500

export function Logs() {
  const [level, setLevel] = useState('')
  const [search, setSearch] = useState('')
  const [sessionId, setSessionId] = useState('')
  const [issueNum, setIssueNum] = useState('')
  const [autoRefresh, setAutoRefresh] = useState(false)
  const [live, setLive] = useState(false)
  const [liveEntries, setLiveEntries] = useState<LogEntry[]>([])
  const [expandedId, setExpandedId] = useState<number | null>(null)
  const [offset, setOffset] = useState(0)
  const [allEntries, setAllEntries] = useState<LogEntry[]>([])

  // Active filter state (applied on Search click)
  const [activeFilters, setActiveFilters] = useState({
    level: '',
    search: '',
    sessionId: '',
    issueNum: '',
  })

  const setOnLogEntry = useWSStore((s) => s.setOnLogEntry)

  // Register/unregister the WS log.entry callback when live mode toggles.
  const handleLiveEntry = useCallback(
    (entry: LogEntry) => {
      // Client-side filter: apply active level and search filters.
      if (activeFilters.level && entry.level !== activeFilters.level) return
      if (
        activeFilters.search &&
        !entry.msg.toLowerCase().includes(activeFilters.search.toLowerCase())
      )
        return
      if (activeFilters.sessionId && entry.session_id !== activeFilters.sessionId) return
      if (activeFilters.issueNum && entry.issue_number !== Number(activeFilters.issueNum)) return

      setLiveEntries((prev) => [entry, ...prev].slice(0, LIVE_CAP))
    },
    [activeFilters],
  )

  useEffect(() => {
    if (live) {
      setOnLogEntry(handleLiveEntry)
    } else {
      setOnLogEntry(null)
    }
    return () => {
      setOnLogEntry(null)
    }
  }, [live, handleLiveEntry, setOnLogEntry])

  const logs = useQuery({
    queryKey: [
      'logs',
      activeFilters.level,
      activeFilters.search,
      activeFilters.sessionId,
      activeFilters.issueNum,
      offset,
    ],
    queryFn: async () => {
      const data = await api.getLogs({
        level: activeFilters.level || undefined,
        q: activeFilters.search || undefined,
        session_id: activeFilters.sessionId || undefined,
        issue: activeFilters.issueNum ? Number(activeFilters.issueNum) : undefined,
        limit: PAGE_SIZE,
        offset,
      })
      if (offset > 0) {
        setAllEntries((prev) => [...prev, ...data])
      } else {
        setAllEntries(data)
      }
      return data
    },
    refetchInterval: autoRefresh ? 5_000 : false,
    enabled: !live,
  })

  function handleSearch() {
    setOffset(0)
    setAllEntries([])
    setLiveEntries([])
    setActiveFilters({
      level,
      search,
      sessionId,
      issueNum,
    })
  }

  function handleLoadMore() {
    setOffset((prev) => prev + PAGE_SIZE)
  }

  function handleLiveToggle(enabled: boolean) {
    setLive(enabled)
    if (enabled) {
      setLiveEntries([])
    }
  }

  const displayEntries = live ? liveEntries : allEntries

  return (
    <div>
      <h2 className="mb-4 text-lg font-semibold dark:text-gray-100">Logs</h2>

      {/* Filter bar */}
      <div className="mb-4 flex flex-wrap items-center gap-2">
        <select
          value={level}
          onChange={(e) => setLevel(e.target.value)}
          className="rounded border dark:border-gray-700 bg-white dark:bg-gray-800 px-2 py-1 text-sm dark:text-gray-100"
        >
          <option value="">All levels</option>
          <option value="debug">debug</option>
          <option value="info">info</option>
          <option value="warn">warn</option>
          <option value="error">error</option>
        </select>

        <input
          type="text"
          placeholder="Search messages..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') handleSearch()
          }}
          className="w-48 rounded border dark:border-gray-700 bg-white dark:bg-gray-800 px-2 py-1 text-sm dark:text-gray-100 dark:placeholder-gray-500"
        />

        <input
          type="text"
          placeholder="Session ID"
          value={sessionId}
          onChange={(e) => setSessionId(e.target.value)}
          className="w-36 rounded border dark:border-gray-700 bg-white dark:bg-gray-800 px-2 py-1 text-sm dark:text-gray-100 dark:placeholder-gray-500"
        />

        <input
          type="text"
          placeholder="Issue #"
          value={issueNum}
          onChange={(e) => setIssueNum(e.target.value)}
          className="w-20 rounded border dark:border-gray-700 bg-white dark:bg-gray-800 px-2 py-1 text-sm dark:text-gray-100 dark:placeholder-gray-500"
        />

        <button
          onClick={handleSearch}
          className="rounded bg-blue-600 px-3 py-1 text-sm text-white hover:bg-blue-700"
        >
          Search
        </button>

        <label className="flex items-center gap-1 text-sm text-gray-600 dark:text-gray-400">
          <input
            type="checkbox"
            checked={autoRefresh}
            disabled={live}
            onChange={(e) => setAutoRefresh(e.target.checked)}
          />
          Auto-refresh
        </label>

        <button
          onClick={() => handleLiveToggle(!live)}
          className={`rounded px-3 py-1 text-sm font-medium transition-colors ${
            live
              ? 'bg-green-600 text-white hover:bg-green-700'
              : 'bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-600'
          }`}
        >
          {live ? 'Live \u25cf' : 'Live'}
        </button>

        {live && (
          <span className="text-xs text-gray-500 dark:text-gray-400">
            {liveEntries.length}/{LIVE_CAP} entries
          </span>
        )}
      </div>

      {/* Log table */}
      {!live && logs.isLoading && offset === 0 && (
        <div className="text-sm text-gray-500 dark:text-gray-400">Loading...</div>
      )}
      {!live && logs.error && (
        <div className="text-sm text-red-600 dark:text-red-400">Error: {String(logs.error)}</div>
      )}
      {displayEntries.length > 0 && (
        <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b dark:border-gray-700 bg-gray-50 dark:bg-gray-900 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                <th className="w-28 px-3 py-2">Time</th>
                <th className="w-16 px-3 py-2">Level</th>
                <th className="w-32 px-3 py-2">Component</th>
                <th className="px-3 py-2">Message</th>
              </tr>
            </thead>
            <tbody>
              {displayEntries.map((entry) => (
                <Fragment key={entry.id}>
                  <tr
                    className="cursor-pointer border-b dark:border-gray-700 hover:bg-gray-50 dark:hover:bg-gray-700"
                    onClick={() =>
                      setExpandedId(expandedId === entry.id ? null : entry.id)
                    }
                  >
                    <td
                      className="px-3 py-2 text-xs text-gray-500 dark:text-gray-400"
                      title={entry.ts}
                    >
                      {relativeTime(entry.ts)}
                    </td>
                    <td className="px-3 py-2">
                      <LevelBadge level={entry.level} />
                    </td>
                    <td className="px-3 py-2 text-xs text-gray-600 dark:text-gray-400">
                      {entry.component}
                    </td>
                    <td className="max-w-md truncate px-3 py-2 font-mono text-xs dark:text-gray-300">
                      {entry.msg}
                    </td>
                  </tr>
                  {expandedId === entry.id && entry.fields && (
                    <tr className="border-b dark:border-gray-700 bg-gray-50 dark:bg-gray-900">
                      <td colSpan={4} className="px-3 py-2">
                        <pre className="whitespace-pre-wrap text-xs text-gray-600 dark:text-gray-400">
                          {formatJSON(entry.fields)}
                        </pre>
                      </td>
                    </tr>
                  )}
                </Fragment>
              ))}
            </tbody>
          </table>
          {!live && displayEntries.length > 0 &&
            logs.data &&
            logs.data.length === PAGE_SIZE && (
              <div className="border-t dark:border-gray-700 px-3 py-2 text-center">
                <button
                  onClick={handleLoadMore}
                  className="text-sm text-blue-600 dark:text-blue-400 hover:underline"
                >
                  Load more
                </button>
              </div>
            )}
        </div>
      )}
      {!live && logs.data && displayEntries.length === 0 && (
        <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 py-8 text-center text-sm text-gray-500 dark:text-gray-400">
          No log entries found
        </div>
      )}
      {live && liveEntries.length === 0 && (
        <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 py-8 text-center text-sm text-gray-500 dark:text-gray-400">
          Waiting for log entries...
        </div>
      )}
    </div>
  )
}
