import { useEffect, useRef } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useWSStore } from '../store/wsStore'

const STORAGE_KEY = 'samverk_api_key'
const BASE_DELAY_MS = 1000
const MAX_DELAY_MS = 30_000
const MAX_FAILURES = 10

export function useWebSocket(): void {
  const queryClient = useQueryClient()
  const setConnected = useWSStore((s) => s.setConnected)
  const wsRef = useRef<WebSocket | null>(null)
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const failuresRef = useRef(0)
  const delayRef = useRef(BASE_DELAY_MS)

  useEffect(() => {
    function connect() {
      const token = localStorage.getItem(STORAGE_KEY)
      if (!token) {
        return
      }

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const url = `${protocol}//${window.location.host}/ws?token=${encodeURIComponent(token)}`

      const ws = new WebSocket(url)
      wsRef.current = ws

      ws.onopen = () => {
        failuresRef.current = 0
        delayRef.current = BASE_DELAY_MS
        setConnected(true)
      }

      ws.onmessage = (event: MessageEvent) => {
        try {
          const msg = JSON.parse(event.data as string) as { type: string; data: unknown }
          switch (msg.type) {
            case 'worker.claimed':
              void queryClient.invalidateQueries({ queryKey: ['workers'] })
              void queryClient.invalidateQueries({ queryKey: ['issues'] })
              break
            case 'worker.complete':
              void queryClient.invalidateQueries({ queryKey: ['workers'] })
              void queryClient.invalidateQueries({ queryKey: ['issues'] })
              void queryClient.invalidateQueries({ queryKey: ['sessions'] })
              break
            case 'worker.failed':
              void queryClient.invalidateQueries({ queryKey: ['workers'] })
              void queryClient.invalidateQueries({ queryKey: ['issues'] })
              break
            case 'queue.depth':
              void queryClient.invalidateQueries({ queryKey: ['metrics'] })
              break
            default:
              break
          }
        } catch {
          // Ignore malformed messages
        }
      }

      ws.onclose = () => {
        wsRef.current = null
        setConnected(false)
        failuresRef.current += 1
        if (failuresRef.current >= MAX_FAILURES) {
          return
        }
        timeoutRef.current = setTimeout(() => {
          connect()
        }, delayRef.current)
        delayRef.current = Math.min(delayRef.current * 2, MAX_DELAY_MS)
      }

      ws.onerror = () => {
        // onclose fires after onerror; reconnect logic is handled there
      }
    }

    connect()

    return () => {
      if (timeoutRef.current !== null) {
        clearTimeout(timeoutRef.current)
        timeoutRef.current = null
      }
      if (wsRef.current !== null) {
        wsRef.current.onclose = null
        wsRef.current.close()
        wsRef.current = null
      }
      setConnected(false)
    }
  }, [queryClient, setConnected])
}
