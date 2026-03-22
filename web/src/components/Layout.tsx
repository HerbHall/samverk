import { useState, useCallback } from 'react'
import { Outlet, NavLink, useLocation } from 'react-router'
import { HeaderBar } from './HeaderBar'
import { ChatDrawer } from './ChatDrawer'

const sidebarLinkClass = ({ isActive }: { isActive: boolean }) =>
  `block rounded px-3 py-2 text-sm ${isActive ? 'bg-blue-50 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300 font-medium' : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'}`

const externalLinkClass = ({ isActive }: { isActive: boolean }) =>
  `block rounded px-3 py-2 text-sm ${isActive ? 'bg-purple-50 dark:bg-purple-900/30 text-purple-700 dark:text-purple-300 font-medium' : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'}`

const myQueueLinkClass = ({ isActive }: { isActive: boolean }) =>
  `block rounded px-3 py-2 text-sm ${isActive ? 'bg-amber-50 dark:bg-amber-900/30 text-amber-700 dark:text-amber-300 font-medium' : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'}`

// SVG icon paths for bottom tab bar
function IconDashboard() {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
      <rect x="3" y="3" width="7" height="7" />
      <rect x="14" y="3" width="7" height="7" />
      <rect x="14" y="14" width="7" height="7" />
      <rect x="3" y="14" width="7" height="7" />
    </svg>
  )
}

function IconIssues() {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="10" />
      <line x1="12" y1="8" x2="12" y2="12" />
      <line x1="12" y1="16" x2="12.01" y2="16" />
    </svg>
  )
}

function IconAgents() {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="8" r="4" />
      <path d="M4 20c0-4 3.6-7 8-7s8 3 8 7" />
    </svg>
  )
}

function IconMetrics() {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
      <polyline points="22 12 18 12 15 21 9 3 6 12 2 12" />
    </svg>
  )
}

function IconLogs() {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
      <line x1="8" y1="6" x2="21" y2="6" />
      <line x1="8" y1="12" x2="21" y2="12" />
      <line x1="8" y1="18" x2="21" y2="18" />
      <line x1="3" y1="6" x2="3.01" y2="6" />
      <line x1="3" y1="12" x2="3.01" y2="12" />
      <line x1="3" y1="18" x2="3.01" y2="18" />
    </svg>
  )
}

const bottomTabItems = [
  { to: '/', label: 'Dashboard', icon: <IconDashboard />, end: true },
  { to: '/issues', label: 'Issues', icon: <IconIssues />, end: false },
  { to: '/agents', label: 'Agents', icon: <IconAgents />, end: false },
  { to: '/metrics', label: 'Metrics', icon: <IconMetrics />, end: false },
  { to: '/logs', label: 'Logs', icon: <IconLogs />, end: false },
]

export function Layout() {
  const location = useLocation()
  const isFullPage = location.pathname === '/devkit'
  const [chatOpen, setChatOpen] = useState(false)

  const handleChatToggle = useCallback(() => {
    setChatOpen((prev) => !prev)
  }, [])

  const handleChatClose = useCallback(() => {
    setChatOpen(false)
  }, [])

  return (
    <div className="flex h-screen flex-col bg-gray-50 dark:bg-gray-950">
      <HeaderBar onChatToggle={handleChatToggle} chatOpen={chatOpen} />
      <ChatDrawer open={chatOpen} onClose={handleChatClose} />
      <div className="flex flex-1 overflow-hidden">
        {/* Sidebar -- hidden on mobile, shown on md+ */}
        <aside className="hidden md:block w-56 border-r dark:border-gray-700 bg-white dark:bg-gray-900 p-4">
          <nav className="space-y-1">
            <NavLink to="/" end className={sidebarLinkClass}>
              Dashboard
            </NavLink>
            <NavLink to="/issues" className={sidebarLinkClass}>
              Issues
            </NavLink>
            <NavLink to="/my-queue" className={myQueueLinkClass}>
              My Queue
            </NavLink>
            <NavLink to="/agents" className={sidebarLinkClass}>
              Agents
            </NavLink>
            <NavLink to="/providers" className={sidebarLinkClass}>
              Providers
            </NavLink>
            <NavLink to="/metrics" className={sidebarLinkClass}>
              Metrics
            </NavLink>
            <NavLink to="/logs" className={sidebarLinkClass}>
              Logs
            </NavLink>
            <NavLink
              to="/quality"
              className={({ isActive }) =>
                `block rounded px-3 py-2 text-sm ${isActive ? 'bg-blue-50 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300 font-medium' : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'}`
              }
            >
              Quality
            </NavLink>
            <NavLink
              to="/pipeline"
              className={({ isActive }) =>
                `block rounded px-3 py-2 text-sm ${isActive ? 'bg-blue-50 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300 font-medium' : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'}`
              }
            >
              Pipeline
            </NavLink>
            <NavLink
              to="/mcp"
              className={({ isActive }) =>
                `block rounded px-3 py-2 text-sm ${isActive ? 'bg-blue-50 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300 font-medium' : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'}`
              }
            >
              MCP
            </NavLink>
            <NavLink
              to="/projects"
              className={({ isActive }) =>
                `block rounded px-3 py-2 text-sm ${isActive ? 'bg-blue-50 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300 font-medium' : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'}`
              }
            >
              Projects
            </NavLink>
            <NavLink
              to="/data"
              className={({ isActive }) =>
                `block rounded px-3 py-2 text-sm ${isActive ? 'bg-blue-50 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300 font-medium' : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'}`
              }
            >
              Data
            </NavLink>
            <div className="mt-4 border-t pt-3 dark:border-gray-700">
              <span className="px-3 text-xs font-medium uppercase tracking-wider text-gray-400 dark:text-gray-500">External</span>
            </div>
            <NavLink to="/synapset" className={externalLinkClass}>
              Synapset
            </NavLink>
            <NavLink to="/devkit" className={externalLinkClass}>
              DevKit
            </NavLink>
          </nav>
        </aside>

        {/* Main content -- adds bottom padding on mobile to avoid overlap with tab bar */}
        <main className={`flex-1 overflow-auto pb-16 md:pb-0 ${isFullPage ? '' : 'p-6'}`}>
          <Outlet />
        </main>
      </div>

      {/* Mobile bottom tab bar -- shown only below md breakpoint */}
      <nav className="fixed bottom-0 left-0 right-0 z-50 flex md:hidden border-t border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900">
        {bottomTabItems.map(({ to, label, icon, end }) => (
          <NavLink
            key={to}
            to={to}
            end={end}
            className={({ isActive }) =>
              `flex flex-1 flex-col items-center justify-center gap-0.5 py-2 text-xs ${
                isActive
                  ? 'text-blue-700 dark:text-blue-300'
                  : 'text-gray-500 dark:text-gray-400'
              }`
            }
          >
            {icon}
            <span>{label}</span>
          </NavLink>
        ))}
      </nav>
    </div>
  )
}
