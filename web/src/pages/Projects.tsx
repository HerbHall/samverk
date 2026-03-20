// Route: <Route path="/projects" element={<Projects />} />
// Nav: { path: '/projects', label: 'Projects' }
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import type { ForgeInfo, ProjectInfo } from '../lib/api'

function ForgeTypeBadge({ type }: { type: string }) {
  const colours: Record<string, string> = {
    github: 'bg-gray-800 text-white dark:bg-gray-200 dark:text-gray-900',
    gitea:  'bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200',
  }
  const cls = colours[type] ?? 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300'
  return (
    <span className={`inline-block rounded px-2 py-0.5 text-xs font-semibold ${cls}`}>
      {type}
    </span>
  )
}

function HealthDot({ healthy }: { healthy: boolean }) {
  return (
    <span
      className={`inline-block h-2.5 w-2.5 rounded-full ${healthy ? 'bg-green-500' : 'bg-red-500'}`}
      title={healthy ? 'Healthy' : 'Unhealthy'}
    />
  )
}

function ForgeCard({ forge }: { forge: ForgeInfo }) {
  return (
    <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4 flex flex-col gap-2">
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <HealthDot healthy={forge.healthy} />
          <span className="font-medium text-gray-900 dark:text-gray-100 capitalize">{forge.name}</span>
        </div>
        <ForgeTypeBadge type={forge.type} />
      </div>
      <a
        href={forge.url}
        target="_blank"
        rel="noopener noreferrer"
        className="text-xs text-blue-600 dark:text-blue-400 hover:underline truncate"
      >
        {forge.url}
      </a>
    </div>
  )
}

function StatPill({ label, value, highlight }: { label: string; value: number; highlight?: boolean }) {
  return (
    <div className={`flex flex-col items-center px-3 py-1 rounded ${highlight != null && highlight && value > 0 ? 'bg-amber-50 dark:bg-amber-900/30' : 'bg-gray-50 dark:bg-gray-700'}`}>
      <span className={`text-lg font-bold ${highlight != null && highlight && value > 0 ? 'text-amber-700 dark:text-amber-300' : 'text-gray-900 dark:text-gray-100'}`}>
        {value}
      </span>
      <span className="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{label}</span>
    </div>
  )
}

function ProjectCard({ project }: { project: ProjectInfo }) {
  return (
    <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4 flex flex-col gap-3">
      <div className="flex items-start justify-between gap-2">
        <div className="flex flex-col gap-0.5">
          <span className="font-semibold text-gray-900 dark:text-gray-100">
            {project.name}
            {project.active && (
              <span className="ml-2 text-xs font-normal text-green-600 dark:text-green-400">(active)</span>
            )}
          </span>
          <span className="text-xs text-gray-500 dark:text-gray-400">
            {project.owner}/{project.repo}
          </span>
        </div>
        <div className="flex items-center gap-1.5 shrink-0">
          {project.forge != null && <ForgeTypeBadge type={project.forge} />}
          <span className="text-xs text-gray-400 dark:text-gray-500 capitalize">{project.phase}</span>
        </div>
      </div>

      <div className="flex gap-2">
        <StatPill label="Open" value={project.open_issues} />
        <StatPill label="Needs Human" value={project.needs_human} highlight />
        <StatPill label="In Progress" value={project.in_progress} />
      </div>

      {project.tags != null && project.tags.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {project.tags.map((tag) => (
            <span
              key={tag}
              className="rounded-full bg-gray-100 dark:bg-gray-700 px-2 py-0.5 text-xs text-gray-600 dark:text-gray-300"
            >
              {tag}
            </span>
          ))}
        </div>
      )}
    </div>
  )
}

export function Projects() {
  const forgesQuery = useQuery({
    queryKey: ['forges'],
    queryFn: api.getForges,
    refetchInterval: 30_000,
  })

  const projectsQuery = useQuery({
    queryKey: ['projects'],
    queryFn: api.getProjects,
    refetchInterval: 60_000,
  })

  const forges = forgesQuery.data ?? []
  const projects = projectsQuery.data ?? []

  return (
    <div className="space-y-8 p-6">
      <div>
        <h1 className="text-2xl font-bold text-gray-900 dark:text-gray-100">Projects</h1>
        <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
          Forge health and per-project issue status
        </p>
      </div>

      <section>
        <h2 className="text-lg font-semibold text-gray-800 dark:text-gray-200 mb-3">Forges</h2>
        {forgesQuery.isLoading && (
          <p className="text-sm text-gray-500 dark:text-gray-400">Loading forges…</p>
        )}
        {forgesQuery.isError && (
          <p className="text-sm text-red-600 dark:text-red-400">Failed to load forges.</p>
        )}
        {!forgesQuery.isLoading && forges.length === 0 && (
          <p className="text-sm text-gray-500 dark:text-gray-400">No forges configured.</p>
        )}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {forges.map((forge) => (
            <ForgeCard key={`${forge.type}:${forge.url}`} forge={forge} />
          ))}
        </div>
      </section>

      <section>
        <h2 className="text-lg font-semibold text-gray-800 dark:text-gray-200 mb-3">
          Projects{projects.length > 0 && <span className="ml-2 text-base font-normal text-gray-400">({projects.length})</span>}
        </h2>
        {projectsQuery.isLoading && (
          <p className="text-sm text-gray-500 dark:text-gray-400">Loading projects…</p>
        )}
        {projectsQuery.isError && (
          <p className="text-sm text-red-600 dark:text-red-400">Failed to load projects.</p>
        )}
        {!projectsQuery.isLoading && projects.length === 0 && (
          <p className="text-sm text-gray-500 dark:text-gray-400">No projects registered.</p>
        )}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {projects.map((project) => (
            <ProjectCard key={project.name} project={project} />
          ))}
        </div>
      </section>
    </div>
  )
}
