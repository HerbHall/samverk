import { Routes, Route } from 'react-router'
import { Layout } from './components/Layout'
import { Dashboard } from './pages/Dashboard'
import { Issues } from './pages/Issues'
import { MyQueue } from './pages/MyQueue'
import { Agents } from './pages/Agents'
import { Metrics } from './pages/Metrics'
import { Logs } from './pages/Logs'

export function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<Dashboard />} />
        <Route path="issues" element={<Issues />} />
        <Route path="my-queue" element={<MyQueue />} />
        <Route path="agents" element={<Agents />} />
        <Route path="metrics" element={<Metrics />} />
        <Route path="logs" element={<Logs />} />
      </Route>
    </Routes>
  )
}
