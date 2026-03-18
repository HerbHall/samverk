import { useTheme } from '../hooks/useTheme'

interface HeaderBarProps {
  onChatToggle?: () => void
  chatOpen?: boolean
}

export function HeaderBar({ onChatToggle, chatOpen }: HeaderBarProps) {
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
      <div className="flex items-center gap-2">
        {onChatToggle != null && (
          <button
            onClick={onChatToggle}
            className={`rounded p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-gray-200 ${chatOpen === true ? 'bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400' : ''}`}
            title={chatOpen === true ? 'Close chat' : 'Chat with Claude'}
          >
            {'\u{1F4AC}'}
          </button>
        )}
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
