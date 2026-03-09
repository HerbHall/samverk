import { Outlet, NavLink } from 'react-router'

export function Layout() {
  return (
    <div className="flex h-screen bg-gray-50">
      <aside className="w-56 border-r bg-white p-4">
        <h1 className="mb-6 text-lg font-bold">Samverk</h1>
        <nav className="space-y-1">
          <NavLink
            to="/"
            end
            className={({ isActive }) =>
              `block rounded px-3 py-2 text-sm ${isActive ? 'bg-blue-50 text-blue-700 font-medium' : 'text-gray-700 hover:bg-gray-100'}`
            }
          >
            Dashboard
          </NavLink>
          <NavLink
            to="/issues"
            className={({ isActive }) =>
              `block rounded px-3 py-2 text-sm ${isActive ? 'bg-blue-50 text-blue-700 font-medium' : 'text-gray-700 hover:bg-gray-100'}`
            }
          >
            Issues
          </NavLink>
          <NavLink
            to="/metrics"
            className={({ isActive }) =>
              `block rounded px-3 py-2 text-sm ${isActive ? 'bg-blue-50 text-blue-700 font-medium' : 'text-gray-700 hover:bg-gray-100'}`
            }
          >
            Metrics
          </NavLink>
        </nav>
      </aside>
      <main className="flex-1 overflow-auto p-6">
        <Outlet />
      </main>
    </div>
  )
}
