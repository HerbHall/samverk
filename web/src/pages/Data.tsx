// Route: <Route path="/data" element={<Data />} />
// Nav: { path: '/data', label: 'Data' }

import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import type { DataSource } from '../lib/api'

function formatBytes(bytes: number): string {
  if (bytes >= 1_073_741_824) return `${(bytes / 1_073_741_824).toFixed(2)} GB`
  if (bytes >= 1_048_576) return `${(bytes / 1_048_576).toFixed(1)} MB`
  if (bytes >= 1_024) return `${(bytes / 1_024).toFixed(1)} KB`
  return `${bytes} B`
}

const TYPE_LABELS: Record<string, string> = {
  sqlite: 'SQLite',
  'vector-db': 'Vector DB',
}

const TYPE_COLORS: Record<string, string> = {
  sqlite: 'bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300',
  'vector-db': 'bg-purple-100 dark:bg-purple-900/30 text-purple-700 dark:text-purple-300',
}

function TypeBadge({ type }: { type: string }) {
  const label = TYPE_LABELS[type] ?? type
  const color = TYPE_COLORS[type] ?? 'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300'
  return (
    <span className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${color}`}>
      {label}
    </span>
  )
}

function HealthIndicator({ healthy }: { healthy: boolean }) {
  return (
    <span className="flex items-center gap-1.5">
      <span className={`inline-block h-2 w-2 rounded-full ${healthy ? 'bg-green-500' : 'bg-red-400'}`} />
      <span className={`text-xs font-medium ${healthy ? 'text-green-700 dark:text-green-400' : 'text-red-600 dark:text-red-400'}`}>
        {healthy ? 'Healthy' : 'Unavailable'}
      </span>
    </span>
  )
}

function DataSourceCard({ source }: { source: DataSource }) {
  return (
    <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 p-5">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="truncate text-base font-semibold text-gray-900 dark:text-gray-100">
            {source.name}
          </h3>
          {source.path != null && source.path !== '' && (
            <p className="mt-0.5 truncate text-xs text-gray-400 dark:text-gray-500 font-mono">
              {source.path}
            </p>
          )}
        </div>
        <TypeBadge type={source.type} />
      </div>

      <div className="mt-4 grid grid-cols-2 gap-3">
        <div className="rounded-md bg-gray-50 dark:bg-gray-900 px-3 py-2">
          <p className="text-xs text-gray-500 dark:text-gray-400 uppercase tracking-wide">Size</p>
          <p className="mt-0.5 text-lg font-bold text-gray-900 dark:text-gray-100 font-mono">
            {formatBytes(source.size_bytes)}
          </p>
        </div>
        <div className="rounded-md bg-gray-50 dark:bg-gray-900 px-3 py-2">
          <p className="text-xs text-gray-500 dark:text-gray-400 uppercase tracking-wide">Records</p>
          <p className="mt-0.5 text-lg font-bold text-gray-900 dark:text-gray-100 font-mono">
            {source.record_count > 0 ? source.record_count.toLocaleString() : '—'}
          </p>
        </div>
      </div>

      <div className="mt-3">
        <HealthIndicator healthy={source.healthy} />
      </div>
    </div>
  )
}

function DataSourcesTable({ sources }: { sources: DataSource[] }) {
  const total = sources.reduce((sum, s) => sum + s.size_bytes, 0)

  return (
    <div className="overflow-x-auto rounded-lg border dark:border-gray-700">
      <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700 bg-white dark:bg-gray-800 text-sm">
        <thead className="bg-gray-50 dark:bg-gray-900">
          <tr>
            <th className="px-4 py-3 text-left text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wide">
              Name
            </th>
            <th className="px-4 py-3 text-left text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wide">
              Type
            </th>
            <th className="px-4 py-3 text-right text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wide">
              Size
            </th>
            <th className="px-4 py-3 text-right text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wide">
              Records
            </th>
            <th className="px-4 py-3 text-right text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wide">
              % of Total
            </th>
            <th className="px-4 py-3 text-center text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wide">
              Status
            </th>
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-100 dark:divide-gray-700">
          {sources.map((source) => {
            const pct = total > 0 ? ((source.size_bytes / total) * 100).toFixed(1) : '—'
            return (
              <tr key={source.name} className="hover:bg-gray-50 dark:hover:bg-gray-700/30">
                <td className="px-4 py-3 font-medium text-gray-900 dark:text-gray-100">
                  {source.name}
                </td>
                <td className="px-4 py-3">
                  <TypeBadge type={source.type} />
                </td>
                <td className="px-4 py-3 text-right font-mono text-gray-700 dark:text-gray-300">
                  {formatBytes(source.size_bytes)}
                </td>
                <td className="px-4 py-3 text-right font-mono text-gray-700 dark:text-gray-300">
                  {source.record_count > 0 ? source.record_count.toLocaleString() : '—'}
                </td>
                <td className="px-4 py-3 text-right font-mono text-gray-500 dark:text-gray-400">
                  {pct !== '—' ? `${pct}%` : '—'}
                </td>
                <td className="px-4 py-3 text-center">
                  <HealthIndicator healthy={source.healthy} />
                </td>
              </tr>
            )
          })}
        </tbody>
        {sources.length > 1 && (
          <tfoot className="bg-gray-50 dark:bg-gray-900 border-t dark:border-gray-700">
            <tr>
              <td className="px-4 py-2 text-xs font-semibold text-gray-500 dark:text-gray-400" colSpan={2}>
                Total
              </td>
              <td className="px-4 py-2 text-right font-mono text-xs font-semibold text-gray-700 dark:text-gray-300">
                {formatBytes(total)}
              </td>
              <td className="px-4 py-2 text-right font-mono text-xs text-gray-500 dark:text-gray-400">
                {sources.reduce((sum, s) => sum + s.record_count, 0).toLocaleString()}
              </td>
              <td className="px-4 py-2" colSpan={2} />
            </tr>
          </tfoot>
        )}
      </table>
    </div>
  )
}

export function Data() {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ['data-sources'],
    queryFn: api.getDataSources,
    refetchInterval: 30_000,
  })

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h2 className="text-2xl font-bold text-gray-900 dark:text-gray-100">Data Sources</h2>
        <span className="text-xs text-gray-400 dark:text-gray-500">Auto-refreshes every 30s</span>
      </div>

      {isLoading && (
        <div className="flex items-center justify-center py-20">
          <p className="text-gray-500 dark:text-gray-400">Loading data sources...</p>
        </div>
      )}

      {isError && (
        <div className="rounded-lg border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/30 p-4">
          <p className="font-medium text-red-800 dark:text-red-300">Failed to load data sources</p>
          <p className="mt-1 text-sm text-red-600 dark:text-red-400">{error?.message ?? 'Unknown error'}</p>
        </div>
      )}

      {data != null && data.length === 0 && (
        <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 px-6 py-12 text-center">
          <p className="text-gray-500 dark:text-gray-400">No data sources configured</p>
        </div>
      )}

      {data != null && data.length > 0 && (
        <div className="space-y-6">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {data.map((source) => (
              <DataSourceCard key={source.name} source={source} />
            ))}
          </div>

          <div>
            <h3 className="mb-3 text-sm font-semibold text-gray-700 dark:text-gray-300">
              Storage Overview
            </h3>
            <DataSourcesTable sources={data} />
          </div>
        </div>
      )}
    </div>
  )
}
