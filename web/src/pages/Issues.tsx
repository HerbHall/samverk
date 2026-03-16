import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'

type StateFilter = 'open' | 'closed' | 'all'

function LabelBadge({ label }: { label: string }) {
  return (
    <span className="inline-block rounded-full bg-gray-100 dark:bg-gray-700 px-2 py-0.5 text-xs font-medium text-gray-600 dark:text-gray-300">
      {label}
    </span>
  )
}

function formatDate(dateStr: string): string {
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffMins = Math.floor(diffMs / 60_000)
  const diffHours = Math.floor(diffMs / 3_600_000)
  const diffDays = Math.floor(diffMs / 86_400_000)

  if (diffMins < 1) return 'just now'
  if (diffMins < 60) return `${diffMins}m ago`
  if (diffHours < 24) return `${diffHours}h ago`
  if (diffDays < 30) return `${diffDays}d ago`
  return date.toLocaleDateString()
}

export function Issues() {
  const [stateFilter, setStateFilter] = useState<StateFilter>('open')
  const [searchQuery, setSearchQuery] = useState('')
  const [expandedIssue, setExpandedIssue] = useState<number | null>(null)

  const issues = useQuery({
    queryKey: ['issues', stateFilter],
    queryFn: () => api.listIssues({ state: stateFilter }),
    refetchInterval: 30_000,
  })

  const filteredIssues = useMemo(() => {
    if (!issues.data) return []
    if (searchQuery.trim() === '') return issues.data
    const query = searchQuery.toLowerCase()
    return issues.data.filter(
      (issue) =>
        issue.title.toLowerCase().includes(query) ||
        issue.number.toString().includes(query)
    )
  }, [issues.data, searchQuery])

  const toggleExpand = (issueNumber: number) => {
    setExpandedIssue((prev) => (prev === issueNumber ? null : issueNumber))
  }

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h2 className="text-2xl font-bold text-gray-900 dark:text-gray-100">Issues</h2>
        <span className="text-xs text-gray-400 dark:text-gray-500">Auto-refreshes every 30s</span>
      </div>

      <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center">
        <div className="flex rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800">
          {(['open', 'closed', 'all'] as const).map((state) => (
            <button
              key={state}
              onClick={() => setStateFilter(state)}
              className={`px-4 py-2 text-sm capitalize ${
                stateFilter === state
                  ? 'bg-blue-50 dark:bg-blue-900/30 font-medium text-blue-700 dark:text-blue-300'
                  : 'text-gray-600 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-gray-700'
              } ${state === 'open' ? 'rounded-l-lg' : ''} ${state === 'all' ? 'rounded-r-lg' : ''}`}
            >
              {state}
            </button>
          ))}
        </div>
        <input
          type="text"
          placeholder="Search by title or number..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 px-3 py-2 text-sm text-gray-900 dark:text-gray-100 placeholder-gray-400 dark:placeholder-gray-500 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 sm:w-64"
        />
        {issues.data != null && (
          <span className="text-sm text-gray-500 dark:text-gray-400">
            {filteredIssues.length} of {issues.data.length} issues
          </span>
        )}
      </div>

      {issues.isLoading && (
        <div className="flex items-center justify-center py-20">
          <p className="text-gray-500 dark:text-gray-400">Loading issues...</p>
        </div>
      )}

      {issues.isError && (
        <div className="rounded-lg border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/30 p-4">
          <p className="font-medium text-red-800 dark:text-red-300">Failed to load issues</p>
          <p className="mt-1 text-sm text-red-600 dark:text-red-400">{issues.error?.message}</p>
        </div>
      )}

      {issues.isSuccess && filteredIssues.length === 0 && (
        <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 p-8 text-center">
          <p className="text-gray-500 dark:text-gray-400">
            {searchQuery.trim() !== '' ? 'No issues match your search.' : 'No issues found.'}
          </p>
        </div>
      )}

      {issues.isSuccess && filteredIssues.length > 0 && (
        <div className="overflow-hidden rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b dark:border-gray-700 bg-gray-50 dark:bg-gray-900 text-left text-xs uppercase tracking-wide text-gray-500 dark:text-gray-400">
                <th className="px-4 py-2 w-16">#</th>
                <th className="px-4 py-2">Title</th>
                <th className="px-4 py-2 w-20">State</th>
                <th className="px-4 py-2">Labels</th>
                <th className="px-4 py-2">Assignees</th>
                <th className="px-4 py-2 w-28">Updated</th>
              </tr>
            </thead>
            <tbody>
              {filteredIssues.map((issue) => (
                <IssueRow
                  key={issue.number}
                  issue={issue}
                  expanded={expandedIssue === issue.number}
                  onToggle={() => toggleExpand(issue.number)}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

function IssueRow({
  issue,
  expanded,
  onToggle,
}: {
  issue: { number: number; title: string; body: string; state: string; labels: string[]; assignees: string[]; updated_at: string }
  expanded: boolean
  onToggle: () => void
}) {
  return (
    <>
      <tr
        onClick={onToggle}
        className="cursor-pointer border-b dark:border-gray-700 last:border-b-0 hover:bg-gray-50 dark:hover:bg-gray-700"
      >
        <td className="px-4 py-2.5 font-medium"><a href={`https://github.com/HerbHall/samverk/issues/${issue.number}`} target="_blank" rel="noopener noreferrer" className="text-blue-600 hover:underline dark:text-blue-400" onClick={(e) => e.stopPropagation()}>#{issue.number}</a></td>
        <td className="px-4 py-2.5">
          <div className="flex items-center gap-2">
            <span
              className={`text-xs ${expanded ? 'rotate-90' : ''} inline-block transition-transform dark:text-gray-400`}
            >
              &#9654;
            </span>
            <span className="text-gray-900 dark:text-gray-100">{issue.title}</span>
          </div>
        </td>
        <td className="px-4 py-2.5">
          <span
            className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${
              issue.state === 'open'
                ? 'bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-300'
                : 'bg-purple-100 dark:bg-purple-900/30 text-purple-700 dark:text-purple-300'
            }`}
          >
            {issue.state}
          </span>
        </td>
        <td className="px-4 py-2.5">
          <div className="flex flex-wrap gap-1">
            {issue.labels.map((label) => (
              <LabelBadge key={label} label={label} />
            ))}
          </div>
        </td>
        <td className="px-4 py-2.5 text-gray-600 dark:text-gray-400">
          {issue.assignees.length > 0 ? issue.assignees.join(', ') : '-'}
        </td>
        <td className="px-4 py-2.5 text-gray-500 dark:text-gray-400">{formatDate(issue.updated_at)}</td>
      </tr>
      {expanded && (
        <tr className="bg-gray-50 dark:bg-gray-900">
          <td colSpan={6} className="px-4 py-4">
            <div className="ml-6 max-w-3xl">
              {issue.body.trim() !== '' ? (
                <pre className="whitespace-pre-wrap text-sm text-gray-700 dark:text-gray-300 font-sans leading-relaxed">
                  {issue.body}
                </pre>
              ) : (
                <p className="text-sm italic text-gray-400 dark:text-gray-500">No description provided.</p>
              )}
            </div>
          </td>
        </tr>
      )}
    </>
  )
}
