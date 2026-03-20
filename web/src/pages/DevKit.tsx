import { useQuery } from '@tanstack/react-query'
import { api, type DevkitRuleInfo, type DevkitSkillInfo } from '../lib/api'

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  return `${(bytes / 1024).toFixed(1)} KB`
}

function RulesTable({ rules }: { rules: DevkitRuleInfo[] }) {
  return (
    <div>
      <h2 className="mb-2 text-sm font-semibold uppercase tracking-wide text-zinc-400">
        Rules
      </h2>
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-zinc-700 text-left text-xs text-zinc-500">
            <th className="pb-1 pr-4 font-medium">File</th>
            <th className="pb-1 pr-4 font-medium">Size</th>
            <th className="pb-1 font-medium">Entries</th>
          </tr>
        </thead>
        <tbody>
          {rules.map((rule) => (
            <tr key={rule.name} className="border-b border-zinc-800">
              <td className="py-1.5 pr-4 font-mono text-xs text-zinc-200">
                {rule.name}
              </td>
              <td className="py-1.5 pr-4 text-zinc-400">
                {formatBytes(rule.size_bytes)}
              </td>
              <td className="py-1.5 text-zinc-400">
                {rule.entry_count != null && rule.entry_count > 0
                  ? rule.entry_count
                  : '—'}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function SkillsList({ skills }: { skills: DevkitSkillInfo[] }) {
  return (
    <div>
      <h2 className="mb-2 text-sm font-semibold uppercase tracking-wide text-zinc-400">
        Skills
      </h2>
      <ul className="space-y-1">
        {skills.map((skill) => (
          <li key={skill.name} className="flex items-center gap-2 text-sm">
            <span className="font-mono text-zinc-200">{skill.name}</span>
            {skill.has_workflows && (
              <span className="rounded bg-emerald-900/50 px-1.5 py-0.5 text-xs text-emerald-400">
                workflows
              </span>
            )}
          </li>
        ))}
      </ul>
    </div>
  )
}

function AgentsList({ agents }: { agents: string[] }) {
  return (
    <div className="mt-6">
      <h2 className="mb-2 text-sm font-semibold uppercase tracking-wide text-zinc-400">
        Agents
      </h2>
      <ul className="space-y-1">
        {agents.map((agent) => (
          <li key={agent} className="font-mono text-sm text-zinc-200">
            {agent}
          </li>
        ))}
      </ul>
    </div>
  )
}

export function DevKit() {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ['devkit', 'summary'],
    queryFn: () => api.getDevkitSummary(),
  })

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center text-zinc-400">
        Loading DevKit summary…
      </div>
    )
  }

  if (isError) {
    return (
      <div className="flex h-full items-center justify-center text-red-400">
        Failed to load DevKit summary:{' '}
        {error instanceof Error ? error.message : 'Unknown error'}
      </div>
    )
  }

  if (!data) {
    return (
      <div className="flex h-full items-center justify-center text-zinc-400">
        No data available.
      </div>
    )
  }

  if (!data.configured) {
    return (
      <div className="flex h-full items-center justify-center p-8">
        <div className="max-w-sm rounded-lg border border-zinc-700 bg-zinc-800/50 p-6 text-center">
          <p className="text-sm text-zinc-300">DevKit path not configured.</p>
          <p className="mt-1 text-xs text-zinc-500">
            Set the <span className="font-mono">SAMVERK_DEVKIT_PATH</span>{' '}
            environment variable to enable this view.
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="h-full overflow-y-auto p-6">
      <div className="mb-4 flex items-baseline gap-3">
        <h1 className="text-lg font-semibold text-zinc-100">DevKit</h1>
        <span className="font-mono text-xs text-zinc-500">{data.path}</span>
        <span className="ml-auto text-xs text-zinc-500">
          {data.skill_count ?? 0} skills · {data.agent_count ?? 0} agents
        </span>
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <div className="rounded-lg border border-zinc-700 bg-zinc-800/30 p-4">
          {data.rules && data.rules.length > 0 ? (
            <RulesTable rules={data.rules} />
          ) : (
            <p className="text-sm text-zinc-500">No rules found.</p>
          )}
        </div>

        <div className="rounded-lg border border-zinc-700 bg-zinc-800/30 p-4">
          {data.skills && data.skills.length > 0 && (
            <SkillsList skills={data.skills} />
          )}
          {data.agents && data.agents.length > 0 && (
            <AgentsList agents={data.agents} />
          )}
          {(!data.skills || data.skills.length === 0) &&
            (!data.agents || data.agents.length === 0) && (
              <p className="text-sm text-zinc-500">No skills or agents found.</p>
            )}
        </div>
      </div>
    </div>
  )
}
