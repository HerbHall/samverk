import { useTheme } from '../hooks/useTheme'

export function HeaderBar() {
  const { theme, toggle } = useTheme()
  const version = window.__SAMVERK_VERSION__ || 'dev'
  const commit = window.__SAMVERK_COMMIT__

  return (
    <header className="flex items-center justify-between border-b border-gray-200 bg-white px-6 py-2.5 dark:border-gray-700 dark:bg-gray-900">
      <div className="flex items-center gap-3">
        <span className="text-sm font-bold tracking-wider text-gray-900 dark:text-white">
          SAMVERK DASHBOARD
        </span>
        <span
          className="rounded bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600 dark:bg-gray-800 dark:text-gray-400"
          title={commit ? `Commit: ${commit}` : undefined}
        >
          {version}
        </span>
      </div>
      <div className="flex items-center gap-4 text-sm">
        <a
          href="https://synapset.herbhall.net/dashboard"
          target="_blank"
          rel="noopener noreferrer"
          className="text-gray-500 hover:text-blue-500 dark:text-gray-400 dark:hover:text-blue-400"
        >
          Synapset
        </a>
        <span className="text-gray-300 dark:text-gray-600">|</span>
        <a
          href="https://github.com/HerbHall/devkit"
          target="_blank"
          rel="noopener noreferrer"
          className="text-gray-500 hover:text-blue-500 dark:text-gray-400 dark:hover:text-blue-400"
        >
          DevKit
        </a>
        <span className="text-gray-300 dark:text-gray-600">|</span>
        <button
          onClick={toggle}
          className="rounded p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-gray-200"
          title={theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
        >
          {theme === 'dark' ? '\u2600' : '\u263D'}
        </button>
      </div>
    </header>
  )
}
