import { useState, useCallback } from 'react'

export interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
}

interface ChatResponse {
  content: string
  model: string
  input_tokens: number
  output_tokens: number
}

interface UseChatOptions {
  system?: string
}

export function useChat({ system }: UseChatOptions = {}) {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const sendMessage = useCallback(
    async (content: string) => {
      const userMessage: ChatMessage = { role: 'user', content }
      setMessages((prev) => [...prev, userMessage])
      setIsLoading(true)
      setError(null)

      const allMessages = [...messages, userMessage]

      try {
        const res = await fetch('/api/v1/chat', {
          method: 'POST',
          credentials: 'same-origin',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            messages: allMessages.map((m) => ({ role: m.role, content: m.content })),
            system: system ?? '',
          }),
        })

        if (!res.ok) {
          const body = await res.text()
          throw new Error(`API ${res.status}: ${body}`)
        }

        const data: ChatResponse = await res.json()
        const assistantMessage: ChatMessage = { role: 'assistant', content: data.content }
        setMessages((prev) => [...prev, assistantMessage])
      } catch (err) {
        const msg = err instanceof Error ? err.message : 'Unknown error'
        setError(msg)
      } finally {
        setIsLoading(false)
      }
    },
    [messages, system],
  )

  const clearMessages = useCallback(() => {
    setMessages([])
    setError(null)
  }, [])

  return { messages, isLoading, error, sendMessage, clearMessages }
}
