import { useState, useRef, useEffect, useCallback } from 'react'
import { useChat } from '../hooks/useChat'
import { useDashboardContext, buildSystemPrompt } from '../hooks/useDashboardContext'

interface ChatDrawerProps {
  open: boolean
  onClose: () => void
}

export function ChatDrawer({ open, onClose }: ChatDrawerProps) {
  const [input, setInput] = useState('')
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const ctx = useDashboardContext()
  const systemPrompt = buildSystemPrompt(ctx)

  const { messages, isLoading, error, sendMessage, clearMessages } = useChat({
    system: systemPrompt,
  })

  // Scroll to bottom when messages change.
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  // Focus input when drawer opens.
  useEffect(() => {
    if (open) {
      // Small delay to let the transition finish.
      const timer = setTimeout(() => inputRef.current?.focus(), 150)
      return () => clearTimeout(timer)
    }
  }, [open])

  // Dismiss on Escape key.
  useEffect(() => {
    if (!open) return
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [open, onClose])

  const handleSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault()
      const trimmed = input.trim()
      if (trimmed === '' || isLoading) return
      setInput('')
      sendMessage(trimmed)
    },
    [input, isLoading, sendMessage],
  )

  // Desktop: slide from right. Mobile: slide from bottom.
  const drawerClasses = open
    ? 'translate-x-0 md:translate-x-0'
    : 'translate-y-full md:translate-y-0 md:translate-x-full'

  return (
    <>
      {/* Backdrop (click-outside dismiss) */}
      {open && (
        <div
          className="fixed inset-0 z-40 bg-black/30"
          onClick={onClose}
          role="presentation"
        />
      )}

      {/* Drawer panel */}
      <div
        className={`fixed z-50 flex flex-col bg-white shadow-xl transition-transform duration-300 ease-in-out dark:bg-gray-900 dark:border-gray-700
          bottom-0 left-0 right-0 h-[80vh] rounded-t-xl border-t
          md:bottom-auto md:left-auto md:top-0 md:right-0 md:h-full md:w-[400px] md:rounded-none md:border-l md:border-t-0
          ${drawerClasses}`}
        role="dialog"
        aria-label="Chat with Claude"
      >
        {/* Header */}
        <div className="flex items-center justify-between border-b px-4 py-3 dark:border-gray-700">
          <div className="flex items-center gap-2">
            <span className="text-sm font-semibold text-gray-900 dark:text-white">
              Chat with Claude
            </span>
            <span className="rounded bg-blue-100 px-1.5 py-0.5 text-xs text-blue-700 dark:bg-blue-900/40 dark:text-blue-300">
              {messages.length > 0 ? `${messages.length} msgs` : 'New'}
            </span>
          </div>
          <div className="flex items-center gap-1">
            {messages.length > 0 && (
              <button
                onClick={clearMessages}
                className="rounded p-1 text-xs text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-gray-200"
                title="Clear conversation"
              >
                Clear
              </button>
            )}
            <button
              onClick={onClose}
              className="rounded p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-gray-200"
              title="Close chat"
            >
              {'\u2715'}
            </button>
          </div>
        </div>

        {/* Messages area */}
        <div className="flex-1 overflow-y-auto px-4 py-3">
          {messages.length === 0 && !isLoading && (
            <div className="flex h-full items-center justify-center">
              <div className="text-center text-sm text-gray-400 dark:text-gray-500">
                <p className="mb-2 text-lg">Ask about your system</p>
                <p>Claude has live context about workers, queue, costs, and status.</p>
              </div>
            </div>
          )}

          {messages.map((msg, i) => (
            <div
              key={i}
              className={`mb-3 ${msg.role === 'user' ? 'flex justify-end' : 'flex justify-start'}`}
            >
              <div
                className={`max-w-[85%] rounded-lg px-3 py-2 text-sm whitespace-pre-wrap ${
                  msg.role === 'user'
                    ? 'bg-blue-600 text-white'
                    : 'bg-gray-100 text-gray-900 dark:bg-gray-800 dark:text-gray-100'
                }`}
              >
                {msg.content}
              </div>
            </div>
          ))}

          {isLoading && (
            <div className="mb-3 flex justify-start">
              <div className="rounded-lg bg-gray-100 px-3 py-2 text-sm text-gray-500 dark:bg-gray-800 dark:text-gray-400">
                Thinking...
              </div>
            </div>
          )}

          {error != null && (
            <div className="mb-3 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/30 dark:text-red-300">
              {error}
            </div>
          )}

          <div ref={messagesEndRef} />
        </div>

        {/* Input area */}
        <form onSubmit={handleSubmit} className="border-t px-4 py-3 dark:border-gray-700">
          <div className="flex items-center gap-2">
            <input
              ref={inputRef}
              type="text"
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder="Ask about your dashboard..."
              disabled={isLoading}
              className="flex-1 rounded-lg border bg-white px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-50 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100 dark:placeholder-gray-500 dark:focus:border-blue-400"
            />
            <button
              type="submit"
              disabled={isLoading || input.trim() === ''}
              className="rounded-lg bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
            >
              Send
            </button>
          </div>
        </form>
      </div>
    </>
  )
}
