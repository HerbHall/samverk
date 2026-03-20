import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import { useWSStore } from '../store/wsStore'

function StatusIndicator({ connected, label }: { connected: boolean; label: string }) {
  return (
    <div className="flex items-center gap-2">
      <span
        className={`inline-block h-2.5 w-2.5 rounded-full ${connected ? 'bg-green-500' : 'bg-red-500'}`}
      />
      <span className="text-sm text-gray-700 dark:text-gray-300">{label}</span>
      <span className={`text-xs ${connected ? 'text-green-600' : 'text-red-600'}`}>
        {connected ? 'Connected' : 'Disconnected'}
      </span>
    </div>
  )
}

function StatCard({ title, value, subtitle }: { title: string; value: string; subtitle?: string }) {
  return (
    <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 p-5">
      <p className="text-sm font-medium text-gray-500 dark:text-gray-400">{title}</p>
      <p className="mt-1 text-2xl font-semibold text-gray-900 dark:text-gray-100">{value}</p>
      {subtitle != null && <p className="mt-1 text-xs text-gray-400 dark:text-gray-500">{subtitle}</p>}
    </div>
  )
}

function formatUSD(amount: number): string {
  return `$${amount.toFixed(4)}`
}

function formatTokens(count: number): string {
  if (count >= 1_000_000) {
    return `${(count / 1_000_000).toFixed(1)}M`
  }
  if (count >= 1_000) {
    return `${(count / 1_000).toFixed(1)}K`
  }
  return count.toString()
}

export function Dashboard() {
  const wsConnected = useWSStore((s) => s.connected)

  const status = useQuery({
    queryKey: ['status'],
    queryFn: api.getStatus,
    refetchInterval: wsConnected ? false : 30_000,
  })

  const sessions = useQuery({
    queryKey: ['sessions'],
    queryFn: api.listSessions,
    refetchInterval: wsConnected ? false : 30_000,
  })

  const costs = useQuery({
    queryKey: ['costs'],
    queryFn: api.getCosts,
    refetchInterval: wsConnected ? false : 30_000,
  })

  const isLoading = status.isLoading || sessions.isLoading || costs.isLoading
  const hasError = status.isError || sessions.isError || costs.isError

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-20">
        <p className="text-gray-500 dark:text-gray-400">Loading dashboard...</p>
      </div>
    )
  }

  if (hasError) {
    const errorMsg =
      status.error?.message || sessions.error?.message || costs.error?.message || 'Unknown error'
    return (
      <div className="rounded-lg border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/30 p-4">
        <p className="font-medium text-red-800 dark:text-red-300">Failed to load dashboard</p>
        <p className="mt-1 text-sm text-red-600 dark:text-red-400">{errorMsg}</p>
      </div>
    )
  }

  const activeSessions = sessions.data?.filter((s) => s.status === 'running') ?? []
  const totalSessions = sessions.data?.length ?? 0

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h2 className="text-2xl font-bold text-gray-900 dark:text-gray-100">Dashboard</h2>
        <span className="inline-flex items-center gap-1 text-xs">
          <span className={`h-2 w-2 rounded-full ${wsConnected ? 'bg-green-500' : 'bg-gray-400'}`} />
          <span className="text-gray-500 dark:text-gray-400">
            {wsConnected ? 'Live' : 'Polling (30s)'}
          </span>
        </span>
      </div>

      <section className="mb-8">
        <h3 className="mb-3 text-sm font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
          System Status
        </h3>
        <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
          <div className="flex items-center gap-3 mb-4">
            <span
              className={`inline-block h-3 w-3 rounded-full ${status.data?.healthy ? 'bg-green-500' : 'bg-yellow-500'}`}
            />
            <span className="font-medium text-gray-900 dark:text-gray-100">
              {status.data?.healthy ? 'System Healthy' : 'System Degraded'}
            </span>
          </div>
          <div className="space-y-2">
            <StatusIndicator
              connected={status.data?.forge_connected ?? false}
              label="Git Forge"
            />
            <StatusIndicator
              connected={status.data?.database_connected ?? false}
              label="Database"
            />
          </div>
          <div className="mt-3 border-t dark:border-gray-700 pt-3">
            <p className="text-sm text-gray-500 dark:text-gray-400">
              MCP Tools: <span className="font-medium text-gray-700 dark:text-gray-300">{status.data?.tool_count ?? 0}</span>
            </p>
          </div>
        </div>
      </section>

      <section className="mb-8">
        <h3 className="mb-3 text-sm font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
          Activity
        </h3>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <StatCard
            title="Active Sessions"
            value={activeSessions.length.toString()}
            subtitle={`${totalSessions} total`}
          />
          <StatCard
            title="Total Cost"
            value={formatUSD(costs.data?.total_cost_usd ?? 0)}
            subtitle={`${costs.data?.record_count ?? 0} records`}
          />
          <StatCard
            title="Tokens Used"
            value={formatTokens(
              (costs.data?.total_input_tokens ?? 0) + (costs.data?.total_output_tokens ?? 0)
            )}
            subtitle={`${formatTokens(costs.data?.total_input_tokens ?? 0)} in / ${formatTokens(costs.data?.total_output_tokens ?? 0)} out`}
          />
        </div>
      </section>

      {activeSessions.length > 0 && (
        <section>
          <h3 className="mb-3 text-sm font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
            Running Sessions
          </h3>
          <div className="overflow-hidden rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b dark:border-gray-700 bg-gray-50 dark:bg-gray-900 text-left text-xs uppercase tracking-wide text-gray-500 dark:text-gray-400">
                  <th className="px-4 py-2">Issue</th>
                  <th className="px-4 py-2">Agent</th>
                  <th className="px-4 py-2">Provider</th>
                  <th className="px-4 py-2">Model</th>
                  <th className="px-4 py-2">Started</th>
                </tr>
              </thead>
              <tbody>
                {activeSessions.map((s) => (
                  <tr key={s.id} className="border-b dark:border-gray-700 last:border-b-0">
                    <td className="px-4 py-2 font-medium"><a href={`https://github.com/HerbHall/samverk/issues/${s.issue_number}`} target="_blank" rel="noopener noreferrer" className="text-blue-600 hover:underline dark:text-blue-400">#{s.issue_number}</a></td>
                    <td className="px-4 py-2 dark:text-gray-300">{s.agent_type}</td>
                    <td className="px-4 py-2 dark:text-gray-300">{s.provider}</td>
                    <td className="px-4 py-2 font-mono text-xs dark:text-gray-300">{s.model}</td>
                    <td className="px-4 py-2 text-gray-500 dark:text-gray-400">
                      {new Date(s.started_at).toLocaleString()}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}
    </div>
  )
}
