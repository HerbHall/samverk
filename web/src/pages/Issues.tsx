import { useState, useMemo, useEffect, useCallback } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../lib/api'
import type { Issue } from '../lib/api'

type StateFilter = 'open' | 'closed' | 'all'

const STATUS_ORDER = ['claimed', 'needs-qc', 'needs-human', 'queued', 'blocked', 'other'] as const
type StatusKey = typeof STATUS_ORDER[number]

const DEFAULT_EXPANDED: Record<StatusKey, boolean> = {
  'claimed': true,
  'needs-qc': true,
  'needs-human': true,
  'queued': true,
  'blocked': false,
  'other': false,
}

function formatStatus(status: string): string {
  switch (status) {
    case 'needs-qc': return 'Needs QC'
    case 'needs-human': return 'Needs Human'
    default: return status.charAt(0).toUpperCase() + status.slice(1)
  }
}

function getIssueStatus(labels: string[]): string {
  const statusLabel = labels.find((l) => l.startsWith('status:'))
  return statusLabel ? statusLabel.replace('status:', '') : 'other'
}

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

interface Toast {
  id: number
  message: string
  isError: boolean
}

let toastCounter = 0

export function Issues() {
  const [stateFilter, setStateFilter] = useState<StateFilter>('open')
  const [searchQuery, setSearchQuery] = useState('')
  const [debouncedQuery, setDebouncedQuery] = useState('')
  const [expandedIssue, setExpandedIssue] = useState<number | null>(null)
  const [collapsedGroups, setCollapsedGroups] = useState<Record<string, boolean>>(
    () => Object.fromEntries(STATUS_ORDER.map((s) => [s, !DEFAULT_EXPANDED[s]]))
  )
  const [selectedIssues, setSelectedIssues] = useState<Set<number>>(new Set())
  const [bulkLoading, setBulkLoading] = useState(false)
  const [toasts, setToasts] = useState<Toast[]>([])

  const queryClient = useQueryClient()

  // Debounce the search query by 300ms.
  useEffect(() => {
    const timer = setTimeout(() => setDebouncedQuery(searchQuery), 300)
    return () => clearTimeout(timer)
  }, [searchQuery])

  const isSearching = debouncedQuery.trim() !== ''

  const issues = useQuery({
    queryKey: ['issues', stateFilter],
    queryFn: () => api.listIssues({ state: stateFilter }),
    refetchInterval: 30_000,
    enabled: !isSearching,
  })

  const searchResults = useQuery({
    queryKey: ['issues', 'search', debouncedQuery, stateFilter],
    queryFn: () => api.searchIssues(debouncedQuery, stateFilter !== 'all' ? stateFilter : undefined),
    enabled: isSearching,
  })

  const activeQuery = isSearching ? searchResults : issues

  const filteredIssues = useMemo(() => {
    return activeQuery.data ?? []
  }, [activeQuery.data])

  const grouped = useMemo(() => {
    const groups: Record<StatusKey, Issue[]> = {
      'claimed': [],
      'needs-qc': [],
      'needs-human': [],
      'queued': [],
      'blocked': [],
      'other': [],
    }
    for (const issue of filteredIssues) {
      const status = getIssueStatus(issue.labels ?? [])
      const key = (status in groups ? status : 'other') as StatusKey
      groups[key].push(issue)
    }
    return groups
  }, [filteredIssues])

  const allVisibleNumbers = useMemo(
    () => filteredIssues.map((i) => i.number),
    [filteredIssues]
  )

  const allSelected = allVisibleNumbers.length > 0 && allVisibleNumbers.every((n) => selectedIssues.has(n))

  const toggleExpand = (issueNumber: number) => {
    setExpandedIssue((prev) => (prev === issueNumber ? null : issueNumber))
  }

  const toggleGroup = (status: string) => {
    setCollapsedGroups((prev) => ({ ...prev, [status]: !prev[status] }))
  }

  const toggleSelectIssue = useCallback((num: number) => {
    setSelectedIssues((prev) => {
      const next = new Set(prev)
      if (next.has(num)) {
        next.delete(num)
      } else {
        next.add(num)
      }
      return next
    })
  }, [])

  const toggleSelectAll = useCallback(() => {
    setSelectedIssues((prev) => {
      if (allVisibleNumbers.every((n) => prev.has(n))) {
        return new Set()
      }
      return new Set(allVisibleNumbers)
    })
  }, [allVisibleNumbers])

  const addToast = useCallback((message: string, isError = false) => {
    const id = ++toastCounter
    setToasts((prev) => [...prev, { id, message, isError }])
    setTimeout(() => setToasts((prev) => prev.filter((t) => t.id !== id)), 3000)
    if (!isError) {
      void queryClient.invalidateQueries({ queryKey: ['issues'] })
    }
  }, [queryClient])

  const runBulkAction = useCallback(async (action: 'close' | 'label' | 'assign', value?: string) => {
    const numbers = Array.from(selectedIssues)
    setBulkLoading(true)
    try {
      const result = await api.bulkIssues(action, numbers, value)
      const parts: string[] = []
      if (result.succeeded.length > 0) {
        parts.push(`${result.succeeded.length} succeeded`)
      }
      if (result.failed.length > 0) {
        const nums = result.failed.map((f) => `#${f.number}`).join(', ')
        parts.push(`${result.failed.length} failed (${nums})`)
      }
      addToast(parts.join(', '), result.failed.length > 0)
      setSelectedIssues(new Set())
      await queryClient.invalidateQueries({ queryKey: ['issues'] })
    } catch (err) {
      addToast(err instanceof Error ? err.message : 'Bulk action failed', true)
    } finally {
      setBulkLoading(false)
    }
  }, [selectedIssues, addToast, queryClient])

  const handleClose = useCallback(() => {
    void runBulkAction('close')
  }, [runBulkAction])

  const handleLabel = useCallback(() => {
    const value = window.prompt('Label name to add:')
    if (value != null && value.trim() !== '') {
      void runBulkAction('label', value.trim())
    }
  }, [runBulkAction])

  const handleAssign = useCallback(() => {
    const value = window.prompt('Assignee username:')
    if (value != null && value.trim() !== '') {
      void runBulkAction('assign', value.trim())
    }
  }, [runBulkAction])

  const totalFiltered = filteredIssues.length

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
          placeholder="Search issues..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 px-3 py-2 text-sm text-gray-900 dark:text-gray-100 placeholder-gray-400 dark:placeholder-gray-500 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 sm:w-64"
        />
        {activeQuery.data != null && (
          <span className="text-sm text-gray-500 dark:text-gray-400">
            {totalFiltered} issue{totalFiltered !== 1 ? 's' : ''}
          </span>
        )}
      </div>

      {activeQuery.isLoading && (
        <div className="flex items-center justify-center py-20">
          <p className="text-gray-500 dark:text-gray-400">
            {isSearching ? 'Searching...' : 'Loading issues...'}
          </p>
        </div>
      )}

      {activeQuery.isError && (
        <div className="rounded-lg border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/30 p-4">
          <p className="font-medium text-red-800 dark:text-red-300">Failed to load issues</p>
          <p className="mt-1 text-sm text-red-600 dark:text-red-400">{activeQuery.error?.message}</p>
        </div>
      )}

      {activeQuery.isSuccess && totalFiltered === 0 && (
        <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 p-8 text-center">
          <p className="text-gray-500 dark:text-gray-400">
            {isSearching ? 'No issues match your search.' : 'No issues found.'}
          </p>
        </div>
      )}

      {activeQuery.isSuccess && totalFiltered > 0 && (
        <div className="flex flex-col gap-4">
          {STATUS_ORDER.map((status) => {
            const group = grouped[status]
            if (group.length === 0) return null
            const isCollapsed = collapsedGroups[status]
            const groupNumbers = group.map((i) => i.number)
            const groupAllSelected = groupNumbers.every((n) => selectedIssues.has(n))
            const groupSomeSelected = groupNumbers.some((n) => selectedIssues.has(n))
            return (
              <div key={status} className="overflow-hidden rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800">
                <div
                  className="flex items-center justify-between cursor-pointer px-4 py-3 border-b dark:border-gray-700 bg-gray-50 dark:bg-gray-900 hover:bg-gray-100 dark:hover:bg-gray-700 select-none"
                  onClick={() => toggleGroup(status)}
                >
                  <div className="flex items-center gap-2">
                    <span className={`text-xs inline-block transition-transform dark:text-gray-400 ${isCollapsed ? '' : 'rotate-90'}`}>
                      &#9654;
                    </span>
                    <h2 className="text-sm font-semibold text-gray-700 dark:text-gray-300">
                      {formatStatus(status)}
                    </h2>
                  </div>
                  <span className="rounded-full bg-gray-200 dark:bg-gray-700 px-2 py-0.5 text-xs font-medium text-gray-700 dark:text-gray-300">
                    {group.length}
                  </span>
                </div>
                {!isCollapsed && (
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b dark:border-gray-700 bg-gray-50 dark:bg-gray-900 text-left text-xs uppercase tracking-wide text-gray-500 dark:text-gray-400">
                        <th className="px-4 py-2 w-10">
                          <input
                            type="checkbox"
                            aria-label={`Select all ${formatStatus(status)} issues`}
                            checked={groupAllSelected}
                            ref={(el) => {
                              if (el) el.indeterminate = groupSomeSelected && !groupAllSelected
                            }}
                            onChange={(e) => {
                              e.stopPropagation()
                              setSelectedIssues((prev) => {
                                const next = new Set(prev)
                                if (groupAllSelected) {
                                  groupNumbers.forEach((n) => next.delete(n))
                                } else {
                                  groupNumbers.forEach((n) => next.add(n))
                                }
                                return next
                              })
                            }}
                            onClick={(e) => e.stopPropagation()}
                            className="cursor-pointer"
                          />
                        </th>
                        <th className="px-4 py-2 w-16">#</th>
                        <th className="px-4 py-2">Title</th>
                        <th className="px-4 py-2 w-20">State</th>
                        <th className="px-4 py-2">Labels</th>
                        <th className="px-4 py-2">Assignees</th>
                        <th className="px-4 py-2 w-28">Updated</th>
                      </tr>
                    </thead>
                    <tbody>
                      {group.map((issue) => (
                        <IssueRow
                          key={issue.number}
                          issue={issue}
                          expanded={expandedIssue === issue.number}
                          selected={selectedIssues.has(issue.number)}
                          onToggle={() => toggleExpand(issue.number)}
                          onSelect={() => toggleSelectIssue(issue.number)}
                          onActionComplete={addToast}
                        />
                      ))}
                    </tbody>
                  </table>
                )}
              </div>
            )
          })}
        </div>
      )}

      {/* Global select-all bar — visible when issues are loaded */}
      {activeQuery.isSuccess && totalFiltered > 0 && (
        <div className="mt-3 flex items-center gap-2 text-sm text-gray-500 dark:text-gray-400">
          <input
            type="checkbox"
            aria-label="Select all visible issues"
            checked={allSelected}
            ref={(el) => {
              if (el) {
                const someSelected = allVisibleNumbers.some((n) => selectedIssues.has(n))
                el.indeterminate = someSelected && !allSelected
              }
            }}
            onChange={toggleSelectAll}
            className="cursor-pointer"
          />
          <span>
            {selectedIssues.size > 0
              ? `${selectedIssues.size} of ${totalFiltered} selected`
              : 'Select all'}
          </span>
        </div>
      )}

      {/* Floating bulk action toolbar */}
      {selectedIssues.size > 0 && (
        <div className="fixed bottom-20 left-1/2 -translate-x-1/2 z-50 flex items-center gap-2 rounded-xl border dark:border-gray-600 bg-white dark:bg-gray-800 px-4 py-3 shadow-xl">
          <span className="text-sm font-medium text-gray-700 dark:text-gray-300 mr-2">
            {selectedIssues.size} selected
          </span>
          <button
            onClick={handleClose}
            disabled={bulkLoading}
            className="rounded-lg bg-red-100 dark:bg-red-900/30 px-3 py-1.5 text-sm font-medium text-red-700 dark:text-red-300 hover:bg-red-200 dark:hover:bg-red-800/40 disabled:opacity-50"
          >
            Close
          </button>
          <button
            onClick={handleLabel}
            disabled={bulkLoading}
            className="rounded-lg bg-blue-100 dark:bg-blue-900/30 px-3 py-1.5 text-sm font-medium text-blue-700 dark:text-blue-300 hover:bg-blue-200 dark:hover:bg-blue-800/40 disabled:opacity-50"
          >
            Label
          </button>
          <button
            onClick={handleAssign}
            disabled={bulkLoading}
            className="rounded-lg bg-green-100 dark:bg-green-900/30 px-3 py-1.5 text-sm font-medium text-green-700 dark:text-green-300 hover:bg-green-200 dark:hover:bg-green-800/40 disabled:opacity-50"
          >
            Assign
          </button>
          <button
            onClick={() => setSelectedIssues(new Set())}
            disabled={bulkLoading}
            className="ml-2 rounded-lg px-2 py-1.5 text-sm text-gray-500 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 disabled:opacity-50"
          >
            ✕
          </button>
        </div>
      )}

      {/* Toast notifications */}
      <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2">
        {toasts.map((toast) => (
          <div
            key={toast.id}
            className={`rounded-lg px-4 py-2.5 text-sm text-white shadow-lg transition-all ${
              toast.isError ? 'bg-red-600' : 'bg-gray-800 dark:bg-gray-700'
            }`}
          >
            {toast.message}
          </div>
        ))}
      </div>
    </div>
  )
}

function IssueRow({
  issue,
  expanded,
  selected,
  onToggle,
  onSelect,
  onActionComplete,
}: {
  issue: Issue
  expanded: boolean
  selected: boolean
  onToggle: () => void
  onSelect: () => void
  onActionComplete?: (message: string, isError: boolean) => void
}) {
  const [actionLoading, setActionLoading] = useState(false)
  const isBlocked = issue.labels.includes('status:blocked')

  const handleApprove = async (e: React.MouseEvent) => {
    e.stopPropagation()
    if (actionLoading) return
    setActionLoading(true)
    try {
      await api.approveAction(issue.number)
      onActionComplete?.('Issue approved and moved to queued', false)
    } catch (err) {
      onActionComplete?.(err instanceof Error ? err.message : 'Failed to approve issue', true)
    } finally {
      setActionLoading(false)
    }
  }

  const handleReject = async (e: React.MouseEvent) => {
    e.stopPropagation()
    if (actionLoading) return
    setActionLoading(true)
    try {
      await api.rejectAction(issue.number, 'Rejected from Issues page')
      onActionComplete?.('Issue rejected and closed', false)
    } catch (err) {
      onActionComplete?.(err instanceof Error ? err.message : 'Failed to reject issue', true)
    } finally {
      setActionLoading(false)
    }
  }

  return (
    <>
      <tr
        onClick={onToggle}
        className={`cursor-pointer border-b dark:border-gray-700 last:border-b-0 hover:bg-gray-50 dark:hover:bg-gray-700 ${
          selected ? 'bg-blue-50 dark:bg-blue-900/10' : ''
        }`}
      >
        <td className="px-4 py-2.5">
          <input
            type="checkbox"
            aria-label={`Select issue #${issue.number}`}
            checked={selected}
            onChange={onSelect}
            onClick={(e) => e.stopPropagation()}
            className="cursor-pointer"
          />
        </td>
        <td className="px-4 py-2.5 font-medium">
          <a
            href={`https://github.com/HerbHall/samverk/issues/${issue.number}`}
            target="_blank"
            rel="noopener noreferrer"
            className="text-blue-600 hover:underline dark:text-blue-400"
            onClick={(e) => e.stopPropagation()}
          >
            #{issue.number}
          </a>
        </td>
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
          <td colSpan={7} className="px-4 py-4">
            <div className="ml-6 max-w-3xl">
              {issue.body.trim() !== '' ? (
                <pre className="whitespace-pre-wrap text-sm text-gray-700 dark:text-gray-300 font-sans leading-relaxed">
                  {issue.body}
                </pre>
              ) : (
                <p className="text-sm italic text-gray-400 dark:text-gray-500">No description provided.</p>
              )}
              {isBlocked && (
                <div className="mt-4 flex gap-2">
                  <button
                    onClick={handleApprove}
                    disabled={actionLoading}
                    className="rounded-lg bg-green-100 dark:bg-green-900/30 px-3 py-1.5 text-sm font-medium text-green-700 dark:text-green-300 hover:bg-green-200 dark:hover:bg-green-800/40 disabled:opacity-50"
                  >
                    {actionLoading ? 'Loading...' : 'Approve'}
                  </button>
                  <button
                    onClick={handleReject}
                    disabled={actionLoading}
                    className="rounded-lg bg-red-100 dark:bg-red-900/30 px-3 py-1.5 text-sm font-medium text-red-700 dark:text-red-300 hover:bg-red-200 dark:hover:bg-red-800/40 disabled:opacity-50"
                  >
                    {actionLoading ? 'Loading...' : 'Reject'}
                  </button>
                </div>
              )}
            </div>
          </td>
        </tr>
      )}
    </>
  )
}
