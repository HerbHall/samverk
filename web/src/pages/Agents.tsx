import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import type { ProviderInfo, CapacityInfo } from '../lib/api'

function typeBadgeColor(type: string): string {
  switch (type) {
    case 'claude':
    case 'claude-cli':
      return 'bg-orange-100 text-orange-800'
    case 'ollama':
      return 'bg-purple-100 text-purple-800'
    case 'openai':
      return 'bg-green-100 text-green-800'
    default:
      return 'bg-gray-100 text-gray-700'
  }
}

function routingChainsForProvider(
  providerName: string,
  routingChains: Record<string, string[]>
): string[] {
  const chains: string[] = []
  for (const [chain, providers] of Object.entries(routingChains)) {
    if (providers.includes(providerName)) {
      chains.push(chain)
    }
  }
  return chains
}

function ProviderCard({
  provider,
  routingChains,
}: {
  provider: ProviderInfo
  routingChains: Record<string, string[]>
}) {
  const chains = routingChainsForProvider(provider.name, routingChains)

  return (
    <div className="rounded-lg border bg-white p-5">
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-base font-semibold text-gray-900">{provider.name}</h3>
        <span
          className={`inline-block rounded-full px-2.5 py-0.5 text-xs font-medium ${typeBadgeColor(provider.type)}`}
        >
          {provider.type}
        </span>
      </div>

      <p className="mb-3 font-mono text-sm text-gray-600">{provider.model}</p>

      <div className="flex items-center gap-2 mb-3">
        <span
          className={`inline-block h-2.5 w-2.5 rounded-full ${provider.healthy ? 'bg-green-500' : 'bg-red-500'}`}
        />
        <span className={`text-sm ${provider.healthy ? 'text-green-600' : 'text-red-600'}`}>
          {provider.healthy ? 'Healthy' : 'Unhealthy'}
        </span>
      </div>

      {chains.length > 0 && (
        <div className="mb-3 flex flex-wrap gap-1.5">
          {chains.map((chain) => (
            <span
              key={chain}
              className="inline-block rounded-full bg-blue-50 px-2.5 py-0.5 text-xs font-medium text-blue-700"
            >
              {chain}
            </span>
          ))}
        </div>
      )}

      {provider.account_url != null && provider.account_url !== '' && (
        <a
          href={provider.account_url}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-1 rounded border border-gray-300 px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50"
        >
          Manage Account
          <svg
            className="h-3 w-3"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
            strokeWidth={2}
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"
            />
          </svg>
        </a>
      )}
    </div>
  )
}

function EmptyState() {
  return (
    <div className="rounded-lg border bg-white p-10 text-center">
      <p className="text-sm text-gray-500">No providers configured</p>
      <p className="mt-1 text-xs text-gray-400">
        Add providers to .samverk/providers.yaml to see them here.
      </p>
    </div>
  )
}

export function Agents() {
  const metrics = useQuery({
    queryKey: ['metrics'],
    queryFn: api.getMetrics,
    refetchInterval: 5_000,
  })

  const capacity: CapacityInfo | null = metrics.data?.capacity ?? null

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h2 className="text-2xl font-bold text-gray-900">Agents</h2>
        <span className="text-xs text-gray-400">Auto-refreshes every 5s</span>
      </div>

      {metrics.isLoading && (
        <div className="flex items-center justify-center py-20">
          <p className="text-gray-500">Loading providers...</p>
        </div>
      )}

      {metrics.isError && (
        <div className="rounded-lg border border-red-200 bg-red-50 p-4">
          <p className="font-medium text-red-800">Failed to load provider data</p>
          <p className="mt-1 text-sm text-red-600">{metrics.error?.message ?? 'Unknown error'}</p>
        </div>
      )}

      {metrics.data != null && capacity == null && <EmptyState />}

      {capacity != null && capacity.providers.length === 0 && <EmptyState />}

      {capacity != null && capacity.providers.length > 0 && (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          {capacity.providers.map((p) => (
            <ProviderCard
              key={p.name}
              provider={p}
              routingChains={capacity.routing_chains}
            />
          ))}
        </div>
      )}
    </div>
  )
}
