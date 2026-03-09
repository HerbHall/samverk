import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'

function StatusIndicator({ connected, label }: { connected: boolean; label: string }) {
  return (
    <div className="flex items-center gap-2">
      <span
        className={`inline-block h-2.5 w-2.5 rounded-full ${connected ? 'bg-green-500' : 'bg-red-500'}`}
      />
      <span className="text-sm text-gray-700">{label}</span>
      <span className={`text-xs ${connected ? 'text-green-600' : 'text-red-600'}`}>
        {connected ? 'Connected' : 'Disconnected'}
      </span>
    </div>
  )
}

function ForgeLabel({ name, forgeType }: { name: string; forgeType: string }) {
  const icon = forgeType === 'gitea' ? '??' : '??'
  return (
    <span>
      {icon} {name}{' '}
      <span className="text-xs text-gray-400">({forgeType})</span>
    </span>
  )
}

function StatCard({ title, value, subtitle }: { title: string; value: string; subtitle?: string }) {
  return (
    <div className="rounded-lg border bg-white p-5">
      <p className="text-sm font-medium text-gray-500">{title}</p>
      <p className="mt-1 text-2xl font-semibold text-gray-900">{value}</p>
      {subtitle != null && <p className="mt-1 text-xs text-gray-400">{subtitle}</p>}
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
  const status = useQuery({
    queryKey: ['status'],
    queryFn: api.getStatus,
    refetchInterval: 30_000,
  })

  const sessions = useQuery({
    queryKey: ['sessions'],
    queryFn: api.listSessions,
    refetchInterval: 30_000,
  })

  const costs = useQuery({
    queryKey: ['costs'],
    queryFn: api.getCosts,
    refetchInterval: 30_000,
  })

  const isLoading = status.isLoading || sessions.isLoading || costs.isLoading
  const hasError = status.isError || sessions.isError || costs.isError

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-20">
        <p className="text-gray-500">Loading dashboard...</p>
      </div>
    )
  }

  if (hasError) {
    const errorMsg =
      status.error?.message || sessions.error?.message || costs.error?.message || 'Unknown error'
    return (
      <div className="rounded-lg border border-red-200 bg-red-50 p-4">
        <p className="font-medium text-red-800">Failed to load dashboard</p>
        <p className="mt-1 text-sm text-red-600">{errorMsg}</p>
      </div>
    )
  }

  const activeSessions = sessions.data?.filter((s) => s.status === 'running') ?? []
  const totalSessions = sessions.data?.length ?? 0
  const forges = status.data?.forges ?? []

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h2 className="text-2xl font-bold text-gray-900">Dashboard</h2>
        <span className="text-xs text-gray-400">Auto-refreshes every 30s</span>
      </div>

      <section className="mb-8">
        <h3 className="mb-3 text-sm font-semibold uppercase tracking-wide text-gray-500">
          System Status
        </h3>
        <div className="rounded-lg border bg-white p-4">
          <div className="flex items-center gap-3 mb-4">
            <span
              className={`inline-block h-3 w-3 rounded-full ${status.data?.healthy ? 'bg-green-500' : 'bg-yellow-500'}`}
            />
            <span className="font-medium text-gray-900">
              {status.data?.healthy ? 'System Healthy' : 'System Degraded'}
            </span>
          </div>
          <div className="space-y-2">
            {forges.length > 0 ? (
              forges.map((f) => (
                <StatusIndicator
                  key={f.name}
                  connected={f.connected}
                  label={`${f.name} (${f.forge_type})`}
                />
              ))
            ) : (
              <StatusIndicator
                connected={status.data?.forge_connected ?? false}
                label="Git Forge"
              />
            )}
            <StatusIndicator
              connected={status.data?.database_connected ?? false}
              label="Database"
            />
          </div>
          <div className="mt-3 border-t pt-3">
            <p className="text-sm text-gray-500">
              MCP Tools: <span className="font-medium text-gray-700">{status.data?.tool_count ?? 0}</span>
            </p>
          </div>
        </div>
      </section>

      <section className="mb-8">
        <h3 className="mb-3 text-sm font-semibold uppercase tracking-wide text-gray-500">
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
          <h3 className="mb-3 text-sm font-semibold uppercase tracking-wide text-gray-500">
            Running Sessions
          </h3>
          <div className="overflow-hidden rounded-lg border bg-white">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b bg-gray-50 text-left text-xs uppercase tracking-wide text-gray-500">
                  <th className="px-4 py-2">Issue</th>
                  <th className="px-4 py-2">Agent</th>
                  <th className="px-4 py-2">Provider</th>
                  <th className="px-4 py-2">Model</th>
                  <th className="px-4 py-2">Started</th>
                </tr>
              </thead>
              <tbody>
                {activeSessions.map((s) => (
                  <tr key={s.id} className="border-b last:border-b-0">
                    <td className="px-4 py-2 font-medium">#{s.issue_number}</td>
                    <td className="px-4 py-2">{s.agent_type}</td>
                    <td className="px-4 py-2">{s.provider}</td>
                    <td className="px-4 py-2 font-mono text-xs">{s.model}</td>
                    <td className="px-4 py-2 text-gray-500">
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