import { create } from 'zustand'

interface WSState {
  connected: boolean
  setConnected: (connected: boolean) => void
}

export const useWSStore = create<WSState>()((set) => ({
  connected: false,
  setConnected: (connected) => set({ connected }),
}))
