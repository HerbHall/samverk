import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import type { ProviderHealthEntry } from '../lib/api'

function formatLastSuccess(value: string | undefined): string {
  if (value == null || value === '') return '—'
  if (value === 'never') return 'Never'
  const d = new Date(value)
  if (isNaN(d.getTime())) return value
  return d.toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function ProviderCard({ provider }: { provider: ProviderHealthEntry }) {
  return (
    <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 p-5">
      <div className="flex items-center justify-between mb-2">
        <div className="flex items-center gap-2">
          <span
            className={`inline-block h-2.5 w-2.5 rounded-full ${provider.healthy ? 'bg-green-500' : 'bg-red-500'}`}
          />
          <h3 className="text-base font-semibold text-gray-900 dark:text-gray-100">{provider.name}</h3>
        </div>
        <span
          className={`inline-block rounded-full px-2.5 py-0.5 text-xs font-medium ${provider.healthy ? 'bg-green-100 dark:bg-green-900/30 text-green-800 dark:text-green-300' : 'bg-red-100 dark:bg-red-900/30 text-red-800 dark:text-red-300'}`}
        >
          {provider.healthy ? 'Healthy' : 'Unhealthy'}
        </span>
      </div>

      <div className="space-y-1 text-sm">
        <div className="flex items-center justify-between">
          <span className="text-gray-500 dark:text-gray-400">Last success</span>
          <span className="font-mono text-gray-700 dark:text-gray-300 text-xs">
            {formatLastSuccess(provider.last_success_at)}
          </span>
        </div>

        {provider.model_loaded != null && (
          <div className="flex items-center justify-between">
            <span className="text-gray-500 dark:text-gray-400">Model loaded</span>
            <span className={`text-xs font-medium ${provider.model_loaded ? 'text-green-600 dark:text-green-400' : 'text-gray-500 dark:text-gray-400'}`}>
              {provider.model_loaded ? 'Yes' : 'No'}
            </span>
          </div>
        )}

        {provider.vram_total_bytes != null && provider.vram_total_bytes > 0 && (
          <div className="flex items-center justify-between">
            <span className="text-gray-500 dark:text-gray-400">VRAM free</span>
            <span className="font-mono text-xs text-gray-700 dark:text-gray-300">
              {Math.round((provider.vram_free_bytes ?? 0) / 1024 / 1024 / 1024 * 10) / 10} GB
              {' / '}
              {Math.round(provider.vram_total_bytes / 1024 / 1024 / 1024 * 10) / 10} GB
            </span>
          </div>
        )}
      </div>

      {provider.error != null && provider.error !== '' && (
        <p className="mt-3 text-xs text-red-600 dark:text-red-400 break-words">{provider.error}</p>
      )}
    </div>
  )
}

function RoutingChainsSection({ chains }: { chains: Record<string, string[]> }) {
  const entries = Object.entries(chains)
  if (entries.length === 0) return null

  return (
    <div className="mt-8">
      <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">Routing Chains</h3>
      <div className="space-y-3">
        {entries.map(([chain, providers]) => (
          <div
            key={chain}
            className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 px-5 py-4 flex items-center gap-4 flex-wrap"
          >
            <span className="text-sm font-medium text-gray-700 dark:text-gray-300 w-28 shrink-0">{chain}</span>
            <div className="flex items-center gap-2 flex-wrap">
              {providers.map((name, idx) => (
                <div key={`${name}-${idx}`} className="flex items-center gap-2">
                  {idx > 0 && (
                    <svg className="h-3 w-3 text-gray-400 dark:text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2.5}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
                    </svg>
                  )}
                  <span className="inline-block rounded-full bg-blue-50 dark:bg-blue-900/30 px-2.5 py-0.5 text-xs font-medium text-blue-700 dark:text-blue-300">
                    {name}
                  </span>
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

export function Providers() {
  const health = useQuery({
    queryKey: ['providers'],
    queryFn: api.getProviderHealth,
    refetchInterval: 30_000,
  })

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h2 className="text-2xl font-bold text-gray-900 dark:text-gray-100">Providers</h2>
          {health.data != null && (
            <span className="rounded-full bg-gray-100 dark:bg-gray-700 px-2.5 py-0.5 text-xs font-medium text-gray-600 dark:text-gray-300">
              {health.data.count}
            </span>
          )}
        </div>
        <span className="text-xs text-gray-400 dark:text-gray-500">Auto-refreshes every 30s</span>
      </div>

      {health.isLoading && (
        <div className="flex items-center justify-center py-20">
          <p className="text-gray-500 dark:text-gray-400">Loading providers...</p>
        </div>
      )}

      {health.isError && (
        <div className="rounded-lg border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/30 p-4">
          <p className="font-medium text-red-800 dark:text-red-300">Failed to load provider health</p>
          <p className="mt-1 text-sm text-red-600 dark:text-red-400">{health.error?.message ?? 'Unknown error'}</p>
        </div>
      )}

      {health.data != null && health.data.providers.length === 0 && (
        <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 p-10 text-center">
          <p className="text-sm text-gray-500 dark:text-gray-400">No providers configured</p>
          <p className="mt-1 text-xs text-gray-400 dark:text-gray-500">
            Add providers to .samverk/providers.yaml to see them here.
          </p>
        </div>
      )}

      {health.data != null && health.data.providers.length > 0 && (
        <>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {health.data.providers.map((p) => (
              <ProviderCard key={p.name} provider={p} />
            ))}
          </div>

          {health.data.routing_chains != null && Object.keys(health.data.routing_chains).length > 0 && (
            <RoutingChainsSection chains={health.data.routing_chains} />
          )}
        </>
      )}
    </div>
  )
}
