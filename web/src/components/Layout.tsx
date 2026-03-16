import { Outlet, NavLink } from 'react-router'
import { HeaderBar } from './HeaderBar'

export function Layout() {
  return (
    <div className="flex h-screen flex-col bg-gray-50 dark:bg-gray-950">
      <HeaderBar />
      <div className="flex flex-1 overflow-hidden">
        <aside className="w-56 border-r dark:border-gray-700 bg-white dark:bg-gray-900 p-4">
          <nav className="space-y-1">
            <NavLink
              to="/"
              end
              className={({ isActive }) =>
                `block rounded px-3 py-2 text-sm ${isActive ? 'bg-blue-50 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300 font-medium' : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'}`
              }
            >
              Dashboard
            </NavLink>
            <NavLink
              to="/issues"
              className={({ isActive }) =>
                `block rounded px-3 py-2 text-sm ${isActive ? 'bg-blue-50 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300 font-medium' : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'}`
              }
            >
              Issues
            </NavLink>
            <NavLink
              to="/my-queue"
              className={({ isActive }) =>
                `block rounded px-3 py-2 text-sm ${isActive ? 'bg-amber-50 dark:bg-amber-900/30 text-amber-700 dark:text-amber-300 font-medium' : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'}`
              }
            >
              My Queue
            </NavLink>
            <NavLink
              to="/agents"
              className={({ isActive }) =>
                `block rounded px-3 py-2 text-sm ${isActive ? 'bg-blue-50 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300 font-medium' : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'}`
              }
            >
              Agents
            </NavLink>
            <NavLink
              to="/metrics"
              className={({ isActive }) =>
                `block rounded px-3 py-2 text-sm ${isActive ? 'bg-blue-50 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300 font-medium' : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'}`
              }
            >
              Metrics
            </NavLink>
            <NavLink
              to="/logs"
              className={({ isActive }) =>
                `block rounded px-3 py-2 text-sm ${isActive ? 'bg-blue-50 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300 font-medium' : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'}`
              }
            >
              Logs
            </NavLink>
          </nav>
        </aside>
        <main className="flex-1 overflow-auto p-6">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
