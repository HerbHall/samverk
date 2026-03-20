// Route: <Route path="/mcp" element={<MCP />} />
// Nav: { path: '/mcp', label: 'MCP' }

import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import type { MCPServerInfo } from '../lib/api'

function TypeBadge({ type }: { type: MCPServerInfo['type'] }) {
  const styles =
    type === 'hosted'
      ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
      : 'bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300'
  return (
    <span className={`rounded-full px-2 py-0.5 text-xs font-medium uppercase tracking-wide ${styles}`}>
      {type}
    </span>
  )
}

function HealthDot({ healthy }: { healthy: boolean }) {
  return (
    <span
      className={`inline-block h-2.5 w-2.5 rounded-full ${healthy ? 'bg-green-500' : 'bg-red-500'}`}
      title={healthy ? 'Healthy' : 'Unreachable'}
    />
  )
}

function ServerCard({ server }: { server: MCPServerInfo }) {
  return (
    <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 p-5">
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-center gap-2.5 min-w-0">
          <HealthDot healthy={server.healthy} />
          <h3 className="text-base font-semibold text-gray-900 dark:text-gray-100 truncate">
            {server.name}
          </h3>
          <TypeBadge type={server.type} />
        </div>
        <span className="shrink-0 rounded bg-gray-100 dark:bg-gray-700 px-2 py-0.5 text-xs font-mono text-gray-600 dark:text-gray-300">
          {server.tool_count} tools
        </span>
      </div>

      <p className="mt-2 text-sm text-gray-500 dark:text-gray-400">{server.description}</p>

      <div className="mt-3 flex items-center gap-1.5 text-xs text-gray-400 dark:text-gray-500">
        <span className="font-mono">{server.endpoint}</span>
        <span
          className={`ml-auto font-medium ${server.healthy ? 'text-green-600 dark:text-green-400' : 'text-red-500 dark:text-red-400'}`}
        >
          {server.healthy ? 'reachable' : 'unreachable'}
        </span>
      </div>
    </div>
  )
}

export default function MCP() {
  const servers = useQuery({
    queryKey: ['mcp-servers'],
    queryFn: api.getMCPServers,
    refetchInterval: 30_000,
  })

  return (
    <div className="p-6 max-w-3xl">
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-gray-900 dark:text-gray-100">MCP Servers</h1>
        <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
          Health and tool inventory for hosted and consumed MCP servers.
        </p>
      </div>

      {servers.isLoading && (
        <div className="py-12 text-center text-sm text-gray-400">Loading...</div>
      )}

      {servers.isError && (
        <div className="rounded-lg border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/20 p-4 text-sm text-red-600 dark:text-red-400">
          Failed to load MCP server status.
        </div>
      )}

      {servers.data != null && (
        <div className="space-y-4">
          {servers.data.map((server) => (
            <ServerCard key={server.name} server={server} />
          ))}
        </div>
      )}
    </div>
  )
}
