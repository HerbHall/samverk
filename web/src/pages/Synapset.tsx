import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  LineChart, Line, BarChart, Bar,
  XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer,
} from 'recharts'
import { api } from '../lib/api'
import type {
  SynapsetToolSummaryEntry,
  SynapsetToolHistoryEntry,
} from '../lib/api'

// --- Helpers ---

function formatBytes(bytes: number): string {
  if (bytes >= 1_073_741_824) return `${(bytes / 1_073_741_824).toFixed(1)} GB`
  if (bytes >= 1_048_576) return `${(bytes / 1_048_576).toFixed(1)} MB`
  if (bytes >= 1_024) return `${(bytes / 1_024).toFixed(1)} KB`
  return `${bytes} B`
}

function formatUptime(seconds: number): string {
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (d > 0) return `${d}d ${h}h`
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

// --- Reusable Components ---

function StatCard({ label, value, sub, color }: { label: string; value: string; sub?: string; color?: string }) {
  return (
    <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 px-4 py-3">
      <p className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wide">{label}</p>
      <p className={`mt-1 text-2xl font-bold ${color ?? 'text-gray-900 dark:text-gray-100'}`}>{value}</p>
      {sub != null && <p className="mt-0.5 text-xs text-gray-400 dark:text-gray-500">{sub}</p>}
    </div>
  )
}

function SectionCard({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800">
      <div className="px-4 py-3 border-b dark:border-gray-700 bg-gray-50 dark:bg-gray-900">
        <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300">{title}</h3>
      </div>
      <div className="px-4 py-2">{children}</div>
    </div>
  )
}

function PeriodSelector({ value, onChange }: { value: string; onChange: (p: string) => void }) {
  const periods = [
    { label: '1D', value: '1d' },
    { label: '1W', value: '7d' },
    { label: '1M', value: '30d' },
    { label: '1Y', value: '365d' },
  ]
  return (
    <div className="flex gap-1">
      {periods.map((p) => (
        <button
          key={p.value}
          onClick={() => onChange(p.value)}
          className={`rounded px-2.5 py-1 text-xs font-medium transition-colors ${
            value === p.value
              ? 'bg-purple-600 text-white'
              : 'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-600'
          }`}
        >
          {p.label}
        </button>
      ))}
    </div>
  )
}

// --- Chart Colors ---

const TOOL_COLORS: Record<string, string> = {
  search_memory: '#8b5cf6',
  store_memory: '#3b82f6',
  import_memories: '#10b981',
  delete_memory: '#ef4444',
  list_memories: '#f59e0b',
  search_all: '#ec4899',
  list_pools: '#6366f1',
}

function getToolColor(name: string): string {
  return TOOL_COLORS[name] ?? '#94a3b8'
}

// --- Tool Calls Chart ---

function ToolCallsChart({ data, period }: { data: SynapsetToolHistoryEntry[]; period: string }) {
  // Bucket tool history entries by time interval
  const chartData = useMemo(() => {
    if (data.length === 0) return []

    // Determine bucket size based on period
    const bucketMs = period === '1d' ? 3600_000 : period === '7d' ? 6 * 3600_000 : 24 * 3600_000

    // Collect unique tool names
    const toolNames = [...new Set(data.map((e) => e.ToolName))]

    // Group entries into time buckets
    const buckets = new Map<number, Record<string, number>>()
    for (const entry of data) {
      const ts = new Date(entry.CreatedAt).getTime()
      const bucketKey = Math.floor(ts / bucketMs) * bucketMs
      const existing = buckets.get(bucketKey) ?? {}
      existing[entry.ToolName] = (existing[entry.ToolName] ?? 0) + 1
      buckets.set(bucketKey, existing)
    }

    // Convert to sorted array
    const sorted = [...buckets.entries()]
      .sort((a, b) => a[0] - b[0])
      .map(([ts, counts]) => {
        const point: Record<string, string | number> = {
          time: new Date(ts).toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }),
        }
        for (const tool of toolNames) {
          point[tool] = counts[tool] ?? 0
        }
        return point
      })

    return sorted
  }, [data, period])

  const toolNames = useMemo(() => [...new Set(data.map((e) => e.ToolName))], [data])

  if (chartData.length === 0) {
    return <div className="py-8 text-center text-sm text-gray-400 dark:text-gray-500">No tool call data for this period</div>
  }

  return (
    <ResponsiveContainer width="100%" height={300}>
      <LineChart data={chartData}>
        <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
        <XAxis dataKey="time" tick={{ fontSize: 11 }} stroke="#9ca3af" />
        <YAxis tick={{ fontSize: 11 }} stroke="#9ca3af" />
        <Tooltip
          contentStyle={{ backgroundColor: '#1f2937', border: '1px solid #374151', borderRadius: '8px' }}
          labelStyle={{ color: '#d1d5db' }}
          itemStyle={{ color: '#d1d5db' }}
        />
        <Legend />
        {toolNames.map((tool) => (
          <Line key={tool} type="monotone" dataKey={tool} stroke={getToolColor(tool)} strokeWidth={2} dot={false} />
        ))}
      </LineChart>
    </ResponsiveContainer>
  )
}

// --- Average Latency Chart ---

function LatencyChart({ tools }: { tools: SynapsetToolSummaryEntry[] }) {
  const chartData = useMemo(() => {
    return tools.map((t) => ({
      name: t.ToolName.replace('_', ' '),
      avg: Math.round(t.AvgDurationMs),
      max: Math.round(t.MaxDurationMs),
    }))
  }, [tools])

  if (chartData.length === 0) {
    return <div className="py-8 text-center text-sm text-gray-400 dark:text-gray-500">No latency data</div>
  }

  return (
    <ResponsiveContainer width="100%" height={250}>
      <BarChart data={chartData}>
        <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
        <XAxis dataKey="name" tick={{ fontSize: 11 }} stroke="#9ca3af" />
        <YAxis tick={{ fontSize: 11 }} stroke="#9ca3af" unit="ms" />
        <Tooltip
          contentStyle={{ backgroundColor: '#1f2937', border: '1px solid #374151', borderRadius: '8px' }}
          labelStyle={{ color: '#d1d5db' }}
          itemStyle={{ color: '#d1d5db' }}
        />
        <Legend />
        <Bar dataKey="avg" fill="#8b5cf6" name="Avg (ms)" />
        <Bar dataKey="max" fill="#c084fc" name="Max (ms)" />
      </BarChart>
    </ResponsiveContainer>
  )
}

// --- Search Quality Chart ---

function SearchQualityChart({ distribution }: { distribution: Record<string, number> }) {
  const chartData = useMemo(() => {
    const order = ['0.0-0.3', '0.3-0.5', '0.5-0.7', '0.7-0.9', '0.9-1.0']
    return order.map((range) => ({
      range,
      count: distribution[range] ?? 0,
    }))
  }, [distribution])

  const hasData = chartData.some((d) => d.count > 0)
  if (!hasData) {
    return <div className="py-8 text-center text-sm text-gray-400 dark:text-gray-500">No search data for this period</div>
  }

  return (
    <ResponsiveContainer width="100%" height={250}>
      <BarChart data={chartData}>
        <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
        <XAxis dataKey="range" tick={{ fontSize: 11 }} stroke="#9ca3af" />
        <YAxis tick={{ fontSize: 11 }} stroke="#9ca3af" />
        <Tooltip
          contentStyle={{ backgroundColor: '#1f2937', border: '1px solid #374151', borderRadius: '8px' }}
          labelStyle={{ color: '#d1d5db' }}
          itemStyle={{ color: '#d1d5db' }}
        />
        <Bar dataKey="count" fill="#8b5cf6" name="Searches" radius={[4, 4, 0, 0]} />
      </BarChart>
    </ResponsiveContainer>
  )
}

// --- Main Page ---

export function Synapset() {
  const [period, setPeriod] = useState('1d')

  const status = useQuery({
    queryKey: ['synapset-status'],
    queryFn: api.synapsetStatus,
    refetchInterval: 30_000,
  })

  const toolSummary = useQuery({
    queryKey: ['synapset-tools-summary', period],
    queryFn: () => api.synapsetToolSummary(period),
    refetchInterval: 30_000,
  })

  const toolHistory = useQuery({
    queryKey: ['synapset-tools-history', period],
    queryFn: () => api.synapsetToolHistory(period),
    refetchInterval: 30_000,
  })

  const searchStats = useQuery({
    queryKey: ['synapset-search-stats', period],
    queryFn: () => api.synapsetSearchStats(period),
    refetchInterval: 30_000,
  })

  const poolStats = useQuery({
    queryKey: ['synapset-pool-stats'],
    queryFn: api.synapsetPoolStats,
    refetchInterval: 60_000,
  })

  // Compute derived stats from tool summary
  const totalCalls = toolSummary.data?.reduce((sum, t) => sum + t.CallCount, 0) ?? 0
  const totalErrors = toolSummary.data?.reduce((sum, t) => sum + t.ErrorCount, 0) ?? 0
  const errorRate = totalCalls > 0 ? (totalErrors / totalCalls) * 100 : 0

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h2 className="text-2xl font-bold text-gray-900 dark:text-gray-100">Synapset</h2>
        <PeriodSelector value={period} onChange={setPeriod} />
      </div>

      {status.isError && (
        <div className="rounded-lg border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/30 p-4 mb-6">
          <p className="font-medium text-red-800 dark:text-red-300">Failed to connect to Synapset</p>
          <p className="mt-1 text-sm text-red-600 dark:text-red-400">{status.error?.message ?? 'Unknown error'}</p>
        </div>
      )}

      {/* Status Cards */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4 mb-6">
        <StatCard
          label="Memories"
          value={(status.data?.memory_count ?? 0).toLocaleString()}
          sub={status.data != null ? `DB: ${formatBytes(status.data.db_size_bytes)}` : undefined}
          color="text-purple-700 dark:text-purple-400"
        />
        <StatCard
          label="Pools"
          value={(status.data?.pool_count ?? 0).toString()}
          sub={status.data?.provider ?? undefined}
        />
        <StatCard
          label="Searches / Period"
          value={(searchStats.data?.total_searches ?? 0).toString()}
          sub={searchStats.data != null ? `avg similarity: ${searchStats.data.avg_similarity.toFixed(3)}` : undefined}
        />
        <StatCard
          label="Errors / Period"
          value={totalErrors.toString()}
          sub={`${errorRate.toFixed(1)}% error rate`}
          color={totalErrors > 0 ? 'text-red-600 dark:text-red-400' : 'text-green-600 dark:text-green-400'}
        />
      </div>

      {/* Uptime badge */}
      {status.data != null && (
        <div className="mb-6 flex items-center gap-3 text-sm text-gray-500 dark:text-gray-400">
          <span className="inline-block h-2 w-2 rounded-full bg-green-500" />
          <span>Uptime: {formatUptime(status.data.uptime_seconds)}</span>
          <span className="text-gray-300 dark:text-gray-600">|</span>
          <span className="font-mono text-xs">{status.data.version}</span>
        </div>
      )}

      <div className="space-y-6">
        {/* Tool Calls Chart */}
        <SectionCard title="Tool Calls">
          {toolHistory.isLoading ? (
            <div className="py-8 text-center text-sm text-gray-400">Loading...</div>
          ) : (
            <ToolCallsChart data={toolHistory.data ?? []} period={period} />
          )}
        </SectionCard>

        {/* Latency + Search Quality side by side */}
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <SectionCard title="Average Latency by Tool">
            {toolSummary.isLoading ? (
              <div className="py-8 text-center text-sm text-gray-400">Loading...</div>
            ) : (
              <LatencyChart tools={toolSummary.data ?? []} />
            )}
          </SectionCard>

          <SectionCard title="Search Quality Distribution">
            {searchStats.isLoading ? (
              <div className="py-8 text-center text-sm text-gray-400">Loading...</div>
            ) : (
              <SearchQualityChart distribution={searchStats.data?.similarity_distribution ?? {}} />
            )}
          </SectionCard>
        </div>

        {/* Pool Breakdown */}
        <SectionCard title="Pool Breakdown">
          {poolStats.isLoading ? (
            <div className="py-8 text-center text-sm text-gray-400">Loading...</div>
          ) : poolStats.data != null && poolStats.data.length > 0 ? (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b dark:border-gray-700 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
                    <th className="pb-2 pr-4">Pool</th>
                    <th className="pb-2 pr-4">Model</th>
                    <th className="pb-2 pr-4">Dimensions</th>
                    <th className="pb-2">Last Updated</th>
                  </tr>
                </thead>
                <tbody>
                  {poolStats.data.map((pool) => (
                    <tr key={pool.name} className="border-b dark:border-gray-700 last:border-b-0">
                      <td className="py-2 pr-4 font-medium text-gray-900 dark:text-gray-100">{pool.name}</td>
                      <td className="py-2 pr-4 font-mono text-xs text-gray-500 dark:text-gray-400">{pool.model_name}</td>
                      <td className="py-2 pr-4 text-gray-600 dark:text-gray-400">{pool.dimensions}</td>
                      <td className="py-2 text-xs text-gray-400 dark:text-gray-500">
                        {new Date(pool.updated_at).toLocaleDateString()}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <div className="py-6 text-center text-sm text-gray-400 dark:text-gray-500">No pools configured</div>
          )}
        </SectionCard>
      </div>
    </div>
  )
}
