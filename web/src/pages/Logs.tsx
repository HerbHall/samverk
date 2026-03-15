import { useState, Fragment } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, LogEntry } from '../lib/api'

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
    debug: 'bg-gray-100 text-gray-600',
    info: 'bg-blue-100 text-blue-700',
    warn: 'bg-amber-100 text-amber-800',
    error: 'bg-red-100 text-red-800',
  }
  return (
    <span
      className={`inline-block rounded px-1.5 py-0.5 text-xs font-medium ${colors[level] || 'bg-gray-100 text-gray-600'}`}
    >
      {level}
    </span>
  )
}

const PAGE_SIZE = 100

export function Logs() {
  const [level, setLevel] = useState('')
  const [search, setSearch] = useState('')
  const [sessionId, setSessionId] = useState('')
  const [issueNum, setIssueNum] = useState('')
  const [autoRefresh, setAutoRefresh] = useState(false)
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
  })

  function handleSearch() {
    setOffset(0)
    setAllEntries([])
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

  const displayEntries = allEntries

  return (
    <div>
      <h2 className="mb-4 text-lg font-semibold">Logs</h2>

      {/* Filter bar */}
      <div className="mb-4 flex flex-wrap items-center gap-2">
        <select
          value={level}
          onChange={(e) => setLevel(e.target.value)}
          className="rounded border px-2 py-1 text-sm"
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
          className="w-48 rounded border px-2 py-1 text-sm"
        />

        <input
          type="text"
          placeholder="Session ID"
          value={sessionId}
          onChange={(e) => setSessionId(e.target.value)}
          className="w-36 rounded border px-2 py-1 text-sm"
        />

        <input
          type="text"
          placeholder="Issue #"
          value={issueNum}
          onChange={(e) => setIssueNum(e.target.value)}
          className="w-20 rounded border px-2 py-1 text-sm"
        />

        <button
          onClick={handleSearch}
          className="rounded bg-blue-600 px-3 py-1 text-sm text-white hover:bg-blue-700"
        >
          Search
        </button>

        <label className="flex items-center gap-1 text-sm text-gray-600">
          <input
            type="checkbox"
            checked={autoRefresh}
            onChange={(e) => setAutoRefresh(e.target.checked)}
          />
          Auto-refresh
        </label>
      </div>

      {/* Log table */}
      {logs.isLoading && offset === 0 && (
        <div className="text-sm text-gray-500">Loading...</div>
      )}
      {logs.error && (
        <div className="text-sm text-red-600">Error: {String(logs.error)}</div>
      )}
      {displayEntries.length > 0 && (
        <div className="rounded-lg border bg-white">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-gray-50 text-left text-xs font-medium uppercase text-gray-500">
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
                    className="cursor-pointer border-b hover:bg-gray-50"
                    onClick={() =>
                      setExpandedId(expandedId === entry.id ? null : entry.id)
                    }
                  >
                    <td
                      className="px-3 py-2 text-xs text-gray-500"
                      title={entry.ts}
                    >
                      {relativeTime(entry.ts)}
                    </td>
                    <td className="px-3 py-2">
                      <LevelBadge level={entry.level} />
                    </td>
                    <td className="px-3 py-2 text-xs text-gray-600">
                      {entry.component}
                    </td>
                    <td className="max-w-md truncate px-3 py-2 font-mono text-xs">
                      {entry.msg}
                    </td>
                  </tr>
                  {expandedId === entry.id && entry.fields && (
                    <tr className="border-b bg-gray-50">
                      <td colSpan={4} className="px-3 py-2">
                        <pre className="whitespace-pre-wrap text-xs text-gray-600">
                          {formatJSON(entry.fields)}
                        </pre>
                      </td>
                    </tr>
                  )}
                </Fragment>
              ))}
            </tbody>
          </table>
          {displayEntries.length > 0 &&
            logs.data &&
            logs.data.length === PAGE_SIZE && (
              <div className="border-t px-3 py-2 text-center">
                <button
                  onClick={handleLoadMore}
                  className="text-sm text-blue-600 hover:underline"
                >
                  Load more
                </button>
              </div>
            )}
        </div>
      )}
      {logs.data && displayEntries.length === 0 && (
        <div className="rounded-lg border bg-white py-8 text-center text-sm text-gray-500">
          No log entries found
        </div>
      )}
    </div>
  )
}
