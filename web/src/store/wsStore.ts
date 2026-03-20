import { create } from 'zustand'
import type { LogEntry } from '../lib/api'

interface WSState {
  connected: boolean
  setConnected: (connected: boolean) => void
  onLogEntry: ((entry: LogEntry) => void) | null
  setOnLogEntry: (cb: ((entry: LogEntry) => void) | null) => void
}

export const useWSStore = create<WSState>()((set) => ({
  connected: false,
  setConnected: (connected) => set({ connected }),
  onLogEntry: null,
  setOnLogEntry: (cb) => set({ onLogEntry: cb }),
}))
