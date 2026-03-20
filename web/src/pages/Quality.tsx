// Route: <Route path="/quality" element={<Quality />} />
// Nav: { path: '/quality', label: 'Quality' }

import { useQuery } from '@tanstack/react-query'
import { PieChart, Pie, Cell, Tooltip as RechartTooltip } from 'recharts'
import { api } from '../lib/api'
import type { KPIReport, RootCauseBreakdownEntry } from '../lib/api'

// ── Helpers ───────────────────────────────────────────────────────────────────

function fmtPct(val: number | null): string {
  if (val === null) return 'Insufficient data'
  return `${val.toFixed(1)}%`
}

function fmtMTTF(hours: number | null): string {
  if (hours === null) return 'Insufficient data'
  const h = Math.floor(hours)
  const m = Math.round((hours - h) * 60)
  if (h === 0) return `${m}m`
  if (m === 0) return `${h}h`
  return `${h}h ${m}m`
}

function ftfrColor(val: number | null): string {
  if (val === null) return 'text-gray-500 dark:text-gray-400'
  if (val > 80) return 'text-green-600 dark:text-green-400'
  if (val >= 60) return 'text-amber-600 dark:text-amber-400'
  return 'text-red-600 dark:text-red-400'
}

function ftfrBg(val: number | null): string {
  if (val === null) return 'bg-gray-50 dark:bg-gray-800'
  if (val > 80) return 'bg-green-50 dark:bg-green-900/20'
  if (val >= 60) return 'bg-amber-50 dark:bg-amber-900/20'
  return 'bg-red-50 dark:bg-red-900/20'
}

function recurrenceColor(val: number | null): string {
  if (val === null) return 'text-gray-500 dark:text-gray-400'
  if (val < 10) return 'text-green-600 dark:text-green-400'
  if (val <= 20) return 'text-amber-600 dark:text-amber-400'
  return 'text-red-600 dark:text-red-400'
}

const PIE_COLORS = [
  '#6366f1', // indigo
  '#f59e0b', // amber
  '#10b981', // emerald
  '#ef4444', // red
  '#3b82f6', // blue
  '#8b5cf6', // violet
  '#f97316', // orange
  '#14b8a6', // teal
]

function formatCategory(cat: string): string {
  return cat.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())
}

// ── Sub-components ────────────────────────────────────────────────────────────

function KPICard({
  label,
  value,
  valueClass,
  bgClass,
  sublabel,
}: {
  label: string
  value: string
  valueClass: string
  bgClass: string
  sublabel: string
}) {
  return (
    <div className={`rounded-lg border dark:border-gray-700 p-5 ${bgClass}`}>
      <div className="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
        {label}
      </div>
      <div className={`mt-2 text-3xl font-bold tabular-nums ${valueClass}`}>{value}</div>
      <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">{sublabel}</div>
    </div>
  )
}

function InsufficientBanner({ count }: { count: number }) {
  return (
    <div className="rounded-lg border border-amber-300 bg-amber-50 dark:border-amber-700 dark:bg-amber-900/20 px-4 py-3 text-sm text-amber-800 dark:text-amber-300">
      Insufficient data ({count} event{count === 1 ? '' : 's'}) — KPIs require at least 5 failure events
    </div>
  )
}

function RootCauseChart({ data }: { data: RootCauseBreakdownEntry[] }) {
  if (data.length === 0) {
    return (
      <div className="flex items-center justify-center h-48 text-sm text-gray-500 dark:text-gray-400">
        No root cause data in the last 30 days
      </div>
    )
  }

  const chartData = data.map((d) => ({
    name: formatCategory(d.category),
    value: d.count,
    pct: d.pct,
  }))

  return (
    <div className="flex flex-col items-center gap-4">
      <PieChart width={300} height={260}>
        <Pie
          data={chartData}
          cx={150}
          cy={120}
          innerRadius={60}
          outerRadius={110}
          paddingAngle={2}
          dataKey="value"
        >
          {chartData.map((_entry, index) => (
            <Cell key={`cell-${index}`} fill={PIE_COLORS[index % PIE_COLORS.length]} />
          ))}
        </Pie>
        <RechartTooltip
          formatter={(value, name) => [`${value as number} events`, name]}
        />
      </PieChart>
      <div className="flex flex-wrap justify-center gap-x-4 gap-y-1">
        {chartData.map((d, i) => (
          <div key={d.name} className="flex items-center gap-1.5 text-xs text-gray-600 dark:text-gray-300">
            <span
              className="inline-block h-2.5 w-2.5 rounded-full flex-shrink-0"
              style={{ backgroundColor: PIE_COLORS[i % PIE_COLORS.length] }}
            />
            {d.name} ({d.value})
          </div>
        ))}
      </div>
    </div>
  )
}

function HeadlineCards({ report }: { report: KPIReport }) {
  return (
    <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
      <KPICard
        label="First-Time Fix Rate"
        value={fmtPct(report.first_time_fix_rate)}
        valueClass={ftfrColor(report.first_time_fix_rate)}
        bgClass={ftfrBg(report.first_time_fix_rate)}
        sublabel="Target > 80% · 30d"
      />
      <KPICard
        label="Fix Success Rate"
        value={fmtPct(report.fix_success_rate)}
        valueClass={
          report.fix_success_rate === null
            ? 'text-gray-500 dark:text-gray-400'
            : report.fix_success_rate >= 70
              ? 'text-green-600 dark:text-green-400'
              : 'text-amber-600 dark:text-amber-400'
        }
        bgClass="bg-white dark:bg-gray-800"
        sublabel="Target > 70% · 7d"
      />
      <KPICard
        label="Mean Time to Fix"
        value={fmtMTTF(report.mean_time_to_fix_hours)}
        valueClass={
          report.mean_time_to_fix_hours === null
            ? 'text-gray-500 dark:text-gray-400'
            : report.mean_time_to_fix_hours < 4
              ? 'text-green-600 dark:text-green-400'
              : report.mean_time_to_fix_hours < 8
                ? 'text-amber-600 dark:text-amber-400'
                : 'text-red-600 dark:text-red-400'
        }
        bgClass="bg-white dark:bg-gray-800"
        sublabel="Target < 4h · 7d"
      />
      <KPICard
        label="Recurrence Rate"
        value={fmtPct(report.recurrence_rate)}
        valueClass={recurrenceColor(report.recurrence_rate)}
        bgClass="bg-white dark:bg-gray-800"
        sublabel="Target < 10% · 30d"
      />
    </div>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function Quality() {
  const { data, isLoading, error } = useQuery({
    queryKey: ['quality', 'kpis'],
    queryFn: api.getKPIs,
    refetchInterval: 60_000,
  })

  return (
    <div className="space-y-6 p-4">
      <div>
        <h1 className="text-xl font-bold text-gray-900 dark:text-white">Quality</h1>
        <p className="text-sm text-gray-500 dark:text-gray-400">
          Self-healing system KPIs · auto-refreshes every 60s
        </p>
      </div>

      {isLoading && (
        <div className="text-sm text-gray-500 dark:text-gray-400">Loading KPIs…</div>
      )}

      {error != null && (
        <div className="rounded-lg border border-red-300 bg-red-50 dark:border-red-700 dark:bg-red-900/20 px-4 py-3 text-sm text-red-800 dark:text-red-300">
          Failed to load KPIs: {error instanceof Error ? error.message : 'Unknown error'}
        </div>
      )}

      {data != null && (
        <>
          {data.sample_size < 5 && <InsufficientBanner count={data.sample_size} />}

          {/* Row 1 — Headline KPI Cards */}
          <HeadlineCards report={data} />

          {/* Row 2 — Root Cause Breakdown */}
          <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 p-5">
            <h2 className="text-sm font-semibold text-gray-700 dark:text-gray-200 mb-4">
              Root Cause Breakdown · 30d
            </h2>
            <RootCauseChart data={data.root_cause_breakdown ?? []} />
          </div>

          {/* Row 3 — Advisory placeholder */}
          <div className="rounded-lg border dark:border-gray-700 bg-gray-50 dark:bg-gray-800 p-5">
            <h2 className="text-sm font-semibold text-gray-400 dark:text-gray-500 mb-1">
              Advisory Engine
            </h2>
            <p className="text-xs text-gray-400 dark:text-gray-500">
              Pattern detection recommendations — coming soon (#120)
            </p>
          </div>
        </>
      )}
    </div>
  )
}
