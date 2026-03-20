import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ErrorBoundary } from './App'

// Regression test: without an ErrorBoundary, any render error causes React to unmount
// the entire tree silently, showing a blank screen indistinguishable from a hang.
// This test verifies the ErrorBoundary catches the error and shows a readable message.

describe('ErrorBoundary', () => {
  const ThrowingComponent = () => {
    throw new Error('Test render crash')
  }

  it('shows error message when a child throws', () => {
    const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {})

    render(
      <ErrorBoundary>
        <ThrowingComponent />
      </ErrorBoundary>,
    )

    expect(screen.getByText('Dashboard Error')).toBeInTheDocument()
    expect(screen.getByText('Test render crash')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /reload/i })).toBeInTheDocument()

    consoleSpy.mockRestore()
  })

  it('renders children normally when no error', () => {
    render(
      <ErrorBoundary>
        <div>App content</div>
      </ErrorBoundary>,
    )

    expect(screen.getByText('App content')).toBeInTheDocument()
  })
})
