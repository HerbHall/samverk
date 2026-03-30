import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import type { PipelineStage, PipelineEvent } from '../lib/api'

// ── Helpers ───────────────────────────────────────────────────────────────────

function formatThroughput(rate: number): string {
  return `${rate.toFixed(1)}/hr`
}

function formatCycleTime(hours: number): string {
  if (hours < 24) {
    const h = Math.floor(hours)
    const m = Math.round((hours - h) * 60)
    if (h === 0) return `${m}m`
    if (m === 0) return `${h}h`
    return `${h}h ${m}m`
  }
  const days = Math.floor(hours / 24)
  const remainingH = Math.floor(hours % 24)
  if (remainingH === 0) return `${days}d`
  return `${days}d ${remainingH}h`
}

function formatRelativeTime(ts: string): string {
  const parsed = new Date(ts).getTime()
  if (isNaN(parsed)) return 'unknown'
  const diffMs = Date.now() - parsed
  if (diffMs < 1000) return 'just now'
  const diffS = Math.floor(diffMs / 1000)
  if (diffS < 60) return `${diffS}s ago`
  const diffM = Math.floor(diffS / 60)
  if (diffM < 60) return `${diffM}m ago`
  const diffH = Math.floor(diffM / 60)
  if (diffH < 24) return `${diffH}h ago`
  return `${Math.floor(diffH / 24)}d ago`
}

function stageLabel(name: string): string {
  const labels: Record<string, string> = {
    open: 'Unrouted',
    queued: 'Queued',
    claimed: 'Claimed',
    in_progress: 'In Progress',
    needs_qc: 'Needs QC',
    needs_human: 'Needs Human',
    blocked: 'Blocked',
    done: 'Done',
    failed: 'Failed',
  }
  return labels[name] ?? name
}

function stageTooltip(name: string): string | undefined {
  const tips: Record<string, string> = {
    open: 'Issues with no status label yet — not queued or claimed.',
    queued: 'Issues waiting for an available agent to pick up.',
    claimed: 'An agent has claimed this issue but has not started work.',
    in_progress: 'An agent is actively working on this issue.',
    needs_qc: 'Work is complete, awaiting quality check.',
    needs_human: 'Requires human review or input before proceeding.',
    blocked: 'Blocked by a dependency or external factor.',
    done: 'Closed / completed issues.',
    failed: 'Issues currently in failed state (not historical failure count).',
  }
  return tips[name]
}

// Stages in the primary flow order (left to right)
const PRIMARY_FLOW: string[] = ['open', 'queued', 'claimed', 'in_progress', 'needs_qc', 'done']
// Stages shown as secondary (below primary flow)
const SECONDARY_STAGES: string[] = ['needs_human', 'blocked', 'failed']

// ── Stage Color ───────────────────────────────────────────────────────────────

function stageColors(name: string, count: number): { card: string; badge: string; text: string } {
  if (name === 'needs_human' || name === 'blocked') {
    return {
      card: 'border-red-300 dark:border-red-700 bg-red-50 dark:bg-red-900/20',
      badge: 'bg-red-200 dark:bg-red-800 text-red-900 dark:text-red-200',
      text: 'text-red-800 dark:text-red-300',
    }
  }
  if (name === 'failed') {
    return {
      card: 'border-red-300 dark:border-red-700 bg-red-50 dark:bg-red-900/20',
      badge: 'bg-red-200 dark:bg-red-800 text-red-900 dark:text-red-200',
      text: 'text-red-800 dark:text-red-300',
    }
  }
  if (name === 'done') {
    return {
      card: 'border-green-300 dark:border-green-700 bg-green-50 dark:bg-green-900/20',
      badge: 'bg-green-200 dark:bg-green-800 text-green-900 dark:text-green-200',
      text: 'text-green-800 dark:text-green-300',
    }
  }
  if (count > 5) {
    return {
      card: 'border-amber-300 dark:border-amber-700 bg-amber-50 dark:bg-amber-900/20',
      badge: 'bg-amber-200 dark:bg-amber-800 text-amber-900 dark:text-amber-200',
      text: 'text-amber-800 dark:text-amber-300',
    }
  }
  return {
    card: 'border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800',
    badge: 'bg-blue-100 dark:bg-blue-900/40 text-blue-800 dark:text-blue-200',
    text: 'text-gray-700 dark:text-gray-300',
  }
}

// ── Slide-over Panel ──────────────────────────────────────────────────────────

function StagePanel({
  stage,
  onClose,
}: {
  stage: PipelineStage | null
  onClose: () => void
}) {
  if (stage == null) return null

  return (
    <>
      {/* Overlay */}
      <div
        className="fixed inset-0 z-40 bg-black/40"
        onClick={onClose}
        aria-hidden="true"
      />
      {/* Panel */}
      <aside className="fixed right-0 top-0 z-50 flex h-full w-80 flex-col border-l dark:border-gray-700 bg-white dark:bg-gray-900 shadow-xl">
        <div className="flex items-center justify-between border-b dark:border-gray-700 px-4 py-3">
          <h2 className="text-sm font-semibold text-gray-900 dark:text-gray-100">
            {stageLabel(stage.name)}
            <span className="ml-2 text-xs font-normal text-gray-500 dark:text-gray-400">
              {stage.count} issue{stage.count !== 1 ? 's' : ''}
            </span>
          </h2>
          <button
            className="rounded p-1 text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 hover:text-gray-700 dark:hover:text-gray-200"
            onClick={onClose}
            aria-label="Close panel"
          >
            <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
              <line x1="18" y1="6" x2="6" y2="18" />
              <line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </div>
        <div className="flex-1 overflow-auto p-4">
          {stage.issues.length === 0 ? (
            <p className="text-sm text-gray-400 dark:text-gray-500">No issues in this stage.</p>
          ) : (
            <ul className="space-y-2">
              {stage.issues.map((issue) => (
                <li key={issue.number}>
                  <div
                    className="flex items-start gap-2 rounded-lg border dark:border-gray-700 bg-gray-50 dark:bg-gray-800 px-3 py-2 text-sm hover:border-blue-300 dark:hover:border-blue-600 transition-colors"
                  >
                    <span className="shrink-0 font-semibold text-blue-600 dark:text-blue-400">
                      IS#{issue.number}
                    </span>
                    <span className="text-gray-800 dark:text-gray-200 leading-snug">
                      {issue.title}
                    </span>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      </aside>
    </>
  )
}

// ── Stage Card ────────────────────────────────────────────────────────────────

function StageCard({
  stage,
  onClick,
}: {
  stage: PipelineStage
  onClick: () => void
}) {
  const colors = stageColors(stage.name, stage.count)

  const tip = stageTooltip(stage.name)

  return (
    <button
      className={`rounded-lg border px-3 py-2 text-left transition-colors hover:opacity-80 ${colors.card}`}
      onClick={onClick}
      aria-label={`View ${stageLabel(stage.name)} issues`}
      title={tip}
    >
      <div className={`text-xs font-semibold uppercase tracking-wide ${colors.text}`}>
        {stageLabel(stage.name)}
      </div>
      <div className={`mt-1 inline-block rounded-full px-2 py-0.5 text-sm font-bold ${colors.badge}`}>
        {stage.count}
      </div>
    </button>
  )
}

// ── Arrow ─────────────────────────────────────────────────────────────────────

function Arrow() {
  return (
    <span className="text-gray-400 dark:text-gray-600 select-none shrink-0" aria-hidden="true">
      →
    </span>
  )
}

// ── Metric Card ───────────────────────────────────────────────────────────────

function MetricCard({ label, value, subtitle }: { label: string; value: string; subtitle?: string }) {
  return (
    <div className="rounded-lg border dark:border-gray-700 bg-white dark:bg-gray-800 p-5">
      <p className="text-sm font-medium text-gray-500 dark:text-gray-400">{label}</p>
      <p className="mt-1 text-2xl font-semibold text-gray-900 dark:text-gray-100">{value}</p>
      {subtitle != null && (
        <p className="mt-1 text-xs text-gray-400 dark:text-gray-500">{subtitle}</p>
      )}
    </div>
  )
}

// ── Events Table ──────────────────────────────────────────────────────────────

function EventsTable({ events, isLoading }: { events: PipelineEvent[]; isLoading: boolean }) {
  if (isLoading) {
    return <p className="text-sm text-gray-400 dark:text-gray-500">Loading events...</p>
  }
  if (events.length === 0) {
    return <p className="text-sm text-gray-400 dark:text-gray-500">No pipeline events yet.</p>
  }

  return (
    <div className="overflow-x-auto rounded-lg border dark:border-gray-700">
      <table className="min-w-full text-sm">
        <thead>
          <tr className="border-b dark:border-gray-700 bg-gray-50 dark:bg-gray-800/60">
            <th className="px-4 py-2 text-left text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
              Project
            </th>
            <th className="px-4 py-2 text-left text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
              Issue
            </th>
            <th className="px-4 py-2 text-left text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
              From
            </th>
            <th className="px-4 py-2 text-left text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
              To
            </th>
            <th className="px-4 py-2 text-left text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
              Triggered By
            </th>
            <th className="px-4 py-2 text-right text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
              Time
            </th>
          </tr>
        </thead>
        <tbody>
          {events.map((ev, idx) => (
            <tr
              key={idx}
              className="border-b dark:border-gray-700 last:border-0 bg-white dark:bg-gray-800 hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-colors"
            >
              <td className="px-4 py-2 text-xs text-gray-500 dark:text-gray-400">
                {ev.project || 'samverk'}
              </td>
              <td className="px-4 py-2 font-medium">
                <span className="text-blue-600 dark:text-blue-400">
                  IS#{ev.issue_number}
                </span>
              </td>
              <td className="px-4 py-2 text-gray-600 dark:text-gray-400">
                {stageLabel(ev.from_stage)}
              </td>
              <td className="px-4 py-2 text-gray-900 dark:text-gray-100 font-medium">
                {stageLabel(ev.to_stage)}
              </td>
              <td className="px-4 py-2 text-gray-500 dark:text-gray-400">
                {ev.triggered_by}
              </td>
              <td className="px-4 py-2 text-right text-gray-400 dark:text-gray-500 whitespace-nowrap">
                {formatRelativeTime(ev.timestamp)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

// ── Main Page ─────────────────────────────────────────────────────────────────

export function Pipeline() {
  const [activeStage, setActiveStage] = useState<PipelineStage | null>(null)

  const stages = useQuery({
    queryKey: ['pipeline', 'stages'],
    queryFn: api.getPipelineStages,
    refetchInterval: 30_000,
  })

  const throughput = useQuery({
    queryKey: ['pipeline', 'throughput'],
    queryFn: api.getPipelineThroughput,
    refetchInterval: 30_000,
  })

  const events = useQuery({
    queryKey: ['pipeline', 'events'],
    queryFn: () => api.getPipelineEvents({ limit: 20 }),
    refetchInterval: 30_000,
  })

  const stageMap = new Map<string, PipelineStage>()
  if (stages.data != null) {
    for (const s of stages.data.stages) {
      stageMap.set(s.name, s)
    }
  }

  const primaryStages = PRIMARY_FLOW.map(
    (name) => stageMap.get(name) ?? { name, count: 0, issues: [] },
  )
  const secondaryStages = SECONDARY_STAGES.map(
    (name) => stageMap.get(name) ?? { name, count: 0, issues: [] },
  )

  const totals = stages.data?.totals
  const tp = throughput.data

  if (stages.isError) {
    return (
      <div className="rounded-lg border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/30 p-4">
        <p className="font-medium text-red-800 dark:text-red-300">Failed to load pipeline</p>
        <p className="mt-1 text-sm text-red-600 dark:text-red-400">
          {stages.error instanceof Error ? stages.error.message : 'Unknown error'}
        </p>
      </div>
    )
  }

  return (
    <div>
      <StagePanel stage={activeStage} onClose={() => setActiveStage(null)} />

      <div className="mb-6 flex items-center justify-between">
        <h2 className="text-2xl font-bold text-gray-900 dark:text-gray-100">Pipeline</h2>
        {stages.data != null && (
          <span className="text-xs text-gray-400 dark:text-gray-500">
            Updated {formatRelativeTime(stages.data.generated_at)}
          </span>
        )}
      </div>

      {/* Flowchart */}
      <section className="mb-6">
        <h3 className="mb-3 text-sm font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
          Stage Flow
        </h3>
        {stages.isLoading ? (
          <p className="text-sm text-gray-400 dark:text-gray-500">Loading stages...</p>
        ) : (
          <div className="rounded-lg border dark:border-gray-700 bg-gray-50 dark:bg-gray-900/50 p-4">
            {/* Primary flow */}
            <div className="flex flex-wrap items-center gap-2">
              {primaryStages.map((stage, idx) => (
                <div key={stage.name} className="flex items-center gap-2">
                  <StageCard stage={stage} onClick={() => setActiveStage(stage)} />
                  {idx < primaryStages.length - 1 && <Arrow />}
                </div>
              ))}
            </div>

            {/* Secondary stages */}
            {secondaryStages.some((s) => s.count > 0) && (
              <div className="mt-3 flex flex-wrap items-center gap-2 border-t dark:border-gray-700 pt-3">
                <span className="text-xs text-gray-400 dark:text-gray-500 mr-1">Also:</span>
                {secondaryStages.map((stage) => (
                  <StageCard
                    key={stage.name}
                    stage={stage}
                    onClick={() => setActiveStage(stage)}
                  />
                ))}
              </div>
            )}
          </div>
        )}
      </section>

      {/* Metric Cards */}
      <section className="mb-6">
        <h3 className="mb-3 text-sm font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
          Metrics
        </h3>
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          <MetricCard
            label="Throughput"
            value={tp != null ? formatThroughput(tp.issues_completed_per_hour_24h) : '—'}
            subtitle="issues / hr (24h)"
          />
          <MetricCard
            label="Avg Cycle Time"
            value={tp != null ? formatCycleTime(tp.avg_cycle_time_hours) : '—'}
            subtitle="open → done"
          />
          <MetricCard
            label="Total Open Issues"
            value={totals != null ? String(totals.open) : '—'}
            subtitle="all open issues across all stages"
          />
          <MetricCard
            label="Closed (24h)"
            value={totals != null ? String(totals.throughput_24h) : '—'}
            subtitle="completed last 24h"
          />
        </div>
      </section>

      {/* Recent Events */}
      <section>
        <h3 className="mb-3 text-sm font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
          Recent Pipeline Events
        </h3>
        <EventsTable events={events.data ?? []} isLoading={events.isLoading} />
      </section>
    </div>
  )
}
