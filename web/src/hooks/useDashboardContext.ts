import { useQueryClient } from '@tanstack/react-query'
import type { Session, CostSummary, SystemStatus, MetricsResponse } from '../lib/api'

export interface DashboardContext {
  workers: Session[]
  queueDepth: number
  blockedCount: number
  recentCosts: CostSummary | null
  systemStatus: SystemStatus | null
  activeWorkers: number
  totalWorkers: number
}

function elapsedMinutes(startedAt: string): number {
  const start = new Date(startedAt).getTime()
  const now = Date.now()
  return Math.round((now - start) / 60_000)
}

export function useDashboardContext(): DashboardContext {
  const queryClient = useQueryClient()

  const sessions = queryClient.getQueryData<Session[]>(['sessions'])
  const costs = queryClient.getQueryData<CostSummary>(['costs'])
  const status = queryClient.getQueryData<SystemStatus>(['status'])
  const metrics = queryClient.getQueryData<MetricsResponse>(['metrics'])

  const activeSessions = sessions?.filter((s) => s.status === 'running') ?? []

  return {
    workers: activeSessions,
    queueDepth: metrics?.pool?.queue_depth ?? 0,
    blockedCount: 0, // Not directly available; could be extended later
    recentCosts: costs ?? null,
    systemStatus: status ?? null,
    activeWorkers: metrics?.pool?.active_workers ?? activeSessions.length,
    totalWorkers: metrics?.pool?.total_workers ?? 0,
  }
}

export function buildSystemPrompt(ctx: DashboardContext): string {
  const lines = [
    'You are an assistant for the Samverk autonomous agent dispatcher dashboard.',
    'You have real-time context about the system state shown below.',
    'Answer questions about what is happening, explain problems, and suggest actions.',
    'Be concise -- the user may be on mobile.',
    '',
    `## Current System State (${new Date().toISOString()})`,
    '',
    `System: ${ctx.systemStatus?.healthy ? 'Healthy' : 'DEGRADED'}`,
    `Forge: ${ctx.systemStatus?.forge_connected ? 'Connected' : 'DISCONNECTED'}`,
    `Database: ${ctx.systemStatus?.database_connected ? 'Connected' : 'DISCONNECTED'}`,
    '',
    `Active Workers: ${ctx.activeWorkers}`,
  ]

  if (ctx.workers.length > 0) {
    for (const w of ctx.workers) {
      lines.push(
        `- Issue #${w.issue_number} | ${w.agent_type} | ${w.provider} | running ${elapsedMinutes(w.started_at)}min`,
      )
    }
  } else {
    lines.push('(no active workers)')
  }

  lines.push('')
  lines.push(`Queue Depth: ${ctx.queueDepth} issues waiting`)
  lines.push(`Total Workers: ${ctx.totalWorkers}`)
  lines.push('')

  if (ctx.recentCosts != null) {
    lines.push(`Cost Today: $${ctx.recentCosts.total_cost_usd.toFixed(4)}`)
    lines.push(
      `Tokens: ${ctx.recentCosts.total_input_tokens} in / ${ctx.recentCosts.total_output_tokens} out`,
    )
  }

  return lines.join('\n')
}
