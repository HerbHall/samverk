import { Routes, Route } from 'react-router'
import { Layout } from './components/Layout'
import { Dashboard } from './pages/Dashboard'
import { Issues } from './pages/Issues'

export function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<Dashboard />} />
        <Route path="issues" element={<Issues />} />
      </Route>
    </Routes>
  )
}
