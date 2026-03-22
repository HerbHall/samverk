import React from 'react'
import { Routes, Route } from 'react-router'
import { Layout } from './components/Layout'
import { Dashboard } from './pages/Dashboard'
import { Issues } from './pages/Issues'
import { MyQueue } from './pages/MyQueue'
import { Agents } from './pages/Agents'
import { Metrics } from './pages/Metrics'
import { Logs } from './pages/Logs'
import { Synapset } from './pages/Synapset'
import { DevKit } from './pages/DevKit'
import { Providers } from './pages/Providers'
import MCP from './pages/MCP'
import { Projects } from './pages/Projects'
import { Data } from './pages/Data'
import Quality from './pages/Quality'
import { Pipeline } from './pages/Pipeline'
import { useWebSocket } from './hooks/useWebSocket'

export class ErrorBoundary extends React.Component<
  { children: React.ReactNode },
  { error: Error | null }
> {
  state = { error: null }

  static getDerivedStateFromError(error: Error) {
    return { error }
  }

  componentDidCatch(error: Error) {
    console.error('App crashed:', error)
  }

  render() {
    if (this.state.error) {
      return (
        <div className="flex h-screen items-center justify-center bg-gray-950">
          <div className="max-w-md rounded border border-red-800 bg-gray-900 p-6 text-sm">
            <p className="mb-2 font-semibold text-red-400">Dashboard Error</p>
            <p className="font-mono text-gray-400">{(this.state.error as Error).message}</p>
            <button
              className="mt-4 rounded bg-gray-700 px-3 py-1 text-gray-200 hover:bg-gray-600"
              onClick={() => window.location.reload()}
            >
              Reload
            </button>
          </div>
        </div>
      )
    }
    return this.props.children
  }
}

export function App() {
  useWebSocket()

  return (
    <ErrorBoundary>
      <Routes>
        <Route element={<Layout />}>
          <Route index element={<Dashboard />} />
          <Route path="issues" element={<Issues />} />
          <Route path="my-queue" element={<MyQueue />} />
          <Route path="agents" element={<Agents />} />
          <Route path="providers" element={<Providers />} />
          <Route path="metrics" element={<Metrics />} />
          <Route path="logs" element={<Logs />} />
          <Route path="quality" element={<Quality />} />
          <Route path="pipeline" element={<Pipeline />} />
          <Route path="mcp" element={<MCP />} />
          <Route path="projects" element={<Projects />} />
          <Route path="data" element={<Data />} />
          <Route path="synapset" element={<Synapset />} />
          <Route path="devkit" element={<DevKit />} />
        </Route>
      </Routes>
    </ErrorBoundary>
  )
}
