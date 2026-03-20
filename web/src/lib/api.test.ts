import { describe, it, expect, vi, beforeEach } from 'vitest'
import { api } from './api'

function mockFetch(data: unknown, status = 200) {
  globalThis.fetch = vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(data),
    text: () => Promise.resolve(String(data)),
  })
}

beforeEach(() => {
  vi.restoreAllMocks()
})

// ─── listIssues ────────────────────────────────────────────────────────────────
// Regression test: PR #705 — listIssues was typed Issue[] but API returns {issues, total}.
// forEach() on the object caused a black screen. This test ensures unwrapping never breaks.

describe('listIssues', () => {
  it('unwraps issues array from wrapped response', async () => {
    mockFetch({ issues: [{ number: 1, title: 'Test issue' }], total: 1 })
    const result = await api.listIssues()
    expect(Array.isArray(result)).toBe(true)
    expect('issues' in result).toBe(false) // must NOT be the wrapper object
    expect(result[0].number).toBe(1)
  })

  it('returns empty array when issues is empty', async () => {
    mockFetch({ issues: [], total: 0 })
    const result = await api.listIssues()
    expect(result).toEqual([])
  })
})

// ─── getCosts ─────────────────────────────────────────────────────────────────
// Regression test: CostSummary TypeScript type had wrong field names (total_cost_usd etc.)
// that never matched the Go backend fields (tokens_used, estimated_cost_usd).
// Cost stats showed $0.00 for the entire project lifetime.

describe('getCosts', () => {
  it('field names match Go costResponse struct (tokens_used, estimated_cost_usd)', async () => {
    mockFetch({ tokens_used: 26979, estimated_cost_usd: 0.42, budget_remaining_usd: 9.58 })
    const result = await api.getCosts()
    expect(result.tokens_used).toBe(26979)
    expect(result.estimated_cost_usd).toBe(0.42)
    expect(result.budget_remaining_usd).toBe(9.58)
  })
})

// ─── Array-typed endpoints ────────────────────────────────────────────────────
// Guard: any endpoint typed as T[] must return an array so .forEach() / .filter() / .map()
// don't throw TypeError when the component iterates the result.

describe('array-typed endpoints return arrays', () => {
  it('listSessions returns an array', async () => {
    mockFetch([{ id: 'sess_1', status: 'active' }])
    const result = await api.listSessions()
    expect(Array.isArray(result)).toBe(true)
  })

  it('getActiveWorkers returns an array', async () => {
    mockFetch([{ session_id: 'sess_1', issue_number: 42 }])
    const result = await api.getActiveWorkers()
    expect(Array.isArray(result)).toBe(true)
  })

  it('getLogs returns an array', async () => {
    mockFetch([{ id: 1, ts: '2026-01-01', level: 'info', msg: 'test' }])
    const result = await api.getLogs()
    expect(Array.isArray(result)).toBe(true)
  })
})

// ─── Auth ─────────────────────────────────────────────────────────────────────

describe('auth error handling', () => {
  it('throws Session expired on 401', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      text: () => Promise.resolve('unauthorized'),
    })
    await expect(api.getStatus()).rejects.toThrow('Session expired')
  })

  it('throws API error with status on non-401 failure', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      text: () => Promise.resolve('internal error'),
    })
    await expect(api.getStatus()).rejects.toThrow('API 500')
  })
})
