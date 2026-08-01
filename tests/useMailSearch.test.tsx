// @vitest-environment happy-dom
import { renderHook, waitFor } from '@testing-library/react'
import { expect, test, vi } from 'vitest'

// The network boundary: a server-side search failure now arrives as a real
// error response (HTTP 500), not 200-with-empty-items.
vi.mock('@tinycld/core/lib/pocketbase', () => ({
    pb: { send: vi.fn().mockRejectedValue(new Error('Search failed')) },
}))

import { useMailSearch } from '~/tinycld/mail/hooks/useMailSearch'

// A failed search must surface as an error state, not read as an empty inbox —
// the client half of the swallow that hid the ts.user_org bug.
test('a failed search surfaces an error, not an empty result', async () => {
    const { result } = renderHook(() => useMailSearch('hello'))

    await waitFor(() => expect(result.current.error).not.toBeNull(), { timeout: 3000 })
    expect(result.current.error).toContain('Search failed')
    expect(result.current.isSearching).toBe(false)
    expect(result.current.results).toEqual([])
})
