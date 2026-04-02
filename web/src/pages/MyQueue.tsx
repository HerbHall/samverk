import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, Issue } from '../lib/api'

const HUMAN_LABELS = ['agent:human', 'status:needs-human', 'status:human-pending']
const PRIORITY_ORDER: Record<string, number> = {
  'priority:critical': 0,
  'priority:high': 1,
  'priority:normal': 2,
  'priority:low': 3,
}

function getPriority(labels: string[]): { rank: number; label: string } {
  for (const l of labels) {
    if (l in PRIORITY_ORDER) return { rank: PRIORITY_ORDER[l], label: l }
  }
  return { rank: 99, label: 'unlabeled' }
}

function priorityColor(label: string): string {
  switch (label) {
    case 'priority:critical':
      return 'bg-red-100 dark:bg-red-900/30 text-red-800 dark:text-red-300'
    case 'priority:high':
      return 'bg-orange-100 dark:bg-orange-900/30 text-orange-800 dark:text-orange-300'
    case 'priority:normal':
      return 'bg-yellow-100 dark:bg-yellow-900/30 text-yellow-800 dark:text-yellow-300'
    case 'priority:low':
      return 'bg-green-100 dark:bg-green-900/30 text-green-800 dark:text-green-300'
    default:
      return 'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300'
  }
}

function formatDate(dateStr: string): string {
  const date = new Date(dateStr)
  if (isNaN(date.getTime())) return 'unknown'
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  if (diffMs < 1000) return 'just now'
  const diffMins = Math.floor(diffMs / 60_000)
  const diffHours = Math.floor(diffMs / 3_600_000)
  const diffDays = Math.floor(diffMs / 86_400_000)

  if (diffMins < 1) return 'just now'
  if (diffMins < 60) return `${diffMins}m ago`
  if (diffHours < 24) return `${diffHours}h ago`
  if (diffDays < 30) return `${diffDays}d ago`
  return date.toLocaleDateString()
}

function issueRefLabel(issue: Issue): string {
  const prefix = issue.is_pull_request ? 'PR' : 'IS'
  const project = issue.project_name || ''
  return project ? `${project} ${prefix}#${issue.number}` : `${prefix}#${issue.number}`
}

function issueURL(issue: Issue): string {
  if (issue.forge_url && issue.owner && issue.repo) {
    return `${issue.forge_url}/${issue.owner}/${issue.repo}/issues/${issue.number}`
  }
  // No forge metadata available -- cannot construct URL.
  return '#'
}

function buildAgentPrompt(issue: Issue): string {
  const deps = issue.labels.filter((l) => l.startsWith('status:blocked')).length > 0
    ? '\n\nNote: This issue may have dependencies -- check the issue body for depends_on references.'
    : ''

  const projectRef = issue.project_name && issue.owner && issue.repo
    ? `the ${issue.project_name} project (${issue.owner}/${issue.repo})`
    : 'the Samverk project (d:\\DevSpace\\Samverk)'

  const url = issueURL(issue)

  // Always suggest MCP tool for reading issues (Gitea is primary forge).
  const readCmd = `Use the Samverk MCP get_issue tool (issue #${issue.number}, project: ${issue.project_name ?? issue.repo ?? 'samverk'})`

  return `Resolve issue #${issue.number} in ${projectRef}.

Issue: ${issue.title}
URL: ${url}

Read the full issue body with: ${readCmd}

Follow the Explore -> Plan -> Code -> Verify -> Commit workflow. Create a feature branch, implement the fix, run \`make ci\` to verify, then create a PR using the Samverk MCP \`create_pr\` tool.${deps}`
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)

  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation()
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  return (
    <button
      onClick={handleCopy}
      className={`rounded px-2 py-1 text-xs font-medium transition-colors ${
        copied
          ? 'bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-300'
          : 'bg-blue-50 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300 hover:bg-blue-100 dark:hover:bg-blue-800/40'
      }`}
      title="Copy agent prompt to clipboard"
    >
      {copied ? 'Copied!' : 'Copy prompt'}
    </button>
  )
}

export function MyQueue() {
  const [expandedIssue, setExpandedIssue] = useState<number | null>(null)

  const issues = useQuery({
    queryKey: ['issues', 'open'],
    queryFn: () => api.listIssues({ state: 'open' }),
    refetchInterval: 30_000,
  })

  const humanIssues = useMemo(() => {
    if (!issues.data) return []
    return issues.data
      .filter((issue) =>
        issue.labels.some((l) => HUMAN_LABELS.includes(l))
      )
      .sort((a, b) => {
        const pa = getPriority(a.labels)
        const pb = getPriority(b.labels)
        return pa.rank - pb.rank
      })
  }, [issues.data])

  const toggleExpand = (issueNumber: number) => {
    setExpandedIssue((prev) => (prev === issueNumber ? null : issueNumber))
  }

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold text-gray-900 dark:text-gray-100">My Queue</h2>
          <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
            Issues that need your input -- sorted by priority. Copy the prompt to resolve in an agent session.
          </p>
        </div>
        <div className="flex items-center gap-3">
          <span className="rounded-full bg-amber-100 dark:bg-amber-900/30 px-3 py-1 text-sm font-medium text-amber-800 dark:text-amber-300">
            {humanIssues.length} awaiting
          </span>
          <span className="text-xs text-gray-400 dark:text-gray-500">Auto-refreshes every 30s</span>
        </div>
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

      {issues.isSuccess && humanIssues.length === 0 && (
        <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 p-8 text-center">
          <p className="text-lg font-medium text-gray-700 dark:text-gray-300">Queue empty</p>
          <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">No issues need your attention right now.</p>
        </div>
      )}

      {issues.isSuccess && humanIssues.length > 0 && (
        <div className="space-y-3">
          {humanIssues.map((issue) => {
            const priority = getPriority(issue.labels)
            const expanded = expandedIssue === issue.number
            const prompt = buildAgentPrompt(issue)
            const otherLabels = issue.labels.filter(
              (l) => !HUMAN_LABELS.includes(l) && !(l in PRIORITY_ORDER)
            )

            return (
              <div
                key={issue.number}
                className="overflow-hidden rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800"
              >
                <div
                  onClick={() => toggleExpand(issue.number)}
                  className="flex cursor-pointer items-center gap-4 px-4 py-3 hover:bg-gray-50 dark:hover:bg-gray-700"
                >
                  <span
                    className={`inline-block rounded-full px-2.5 py-0.5 text-xs font-semibold ${priorityColor(priority.label)}`}
                  >
                    {priority.label.replace('priority:', '').toUpperCase()}
                  </span>

                  <a href={issueURL(issue)} target="_blank" rel="noopener noreferrer" className="text-sm font-medium text-blue-600 hover:underline dark:text-blue-400" onClick={(e) => e.stopPropagation()}>{issueRefLabel(issue)}</a>

                  <span className="flex-1 font-medium text-gray-900 dark:text-gray-100">{issue.title}</span>

                  <div className="flex flex-wrap gap-1">
                    {otherLabels.map((label) => (
                      <span
                        key={label}
                        className="inline-block rounded-full bg-gray-100 dark:bg-gray-700 px-2 py-0.5 text-xs text-gray-600 dark:text-gray-300"
                      >
                        {label}
                      </span>
                    ))}
                  </div>

                  <span className="text-xs text-gray-400 dark:text-gray-500 whitespace-nowrap">{formatDate(issue.updated_at)}</span>

                  <CopyButton text={prompt} />

                  <span
                    className={`text-xs text-gray-400 dark:text-gray-500 ${expanded ? 'rotate-90' : ''} inline-block transition-transform`}
                  >
                    &#9654;
                  </span>
                </div>

                {expanded && (
                  <div className="border-t dark:border-gray-700 bg-gray-50 dark:bg-gray-900 px-4 py-4">
                    <div className="grid gap-4 lg:grid-cols-2">
                      <div>
                        <h4 className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
                          Issue Description
                        </h4>
                        <div className="max-h-64 overflow-auto rounded border dark:border-gray-700 bg-white dark:bg-gray-800 p-3">
                          {issue.body.trim() !== '' ? (
                            <pre className="whitespace-pre-wrap text-sm text-gray-700 dark:text-gray-300 font-sans leading-relaxed">
                              {issue.body}
                            </pre>
                          ) : (
                            <p className="text-sm italic text-gray-400 dark:text-gray-500">No description provided.</p>
                          )}
                        </div>
                      </div>

                      <div>
                        <h4 className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
                          Agent Prompt
                        </h4>
                        <div className="relative rounded border dark:border-gray-700 bg-gray-900 p-3">
                          <pre className="whitespace-pre-wrap text-sm text-green-400 font-mono leading-relaxed">
                            {prompt}
                          </pre>
                          <div className="absolute right-2 top-2">
                            <CopyButton text={prompt} />
                          </div>
                        </div>
                        <p className="mt-2 text-xs text-gray-500 dark:text-gray-400">
                          Paste this into a Claude Code session to resolve and complete the issue.
                        </p>
                      </div>
                    </div>
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
