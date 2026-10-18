import { describe, expect, it } from 'vitest'
import { viewerIsOwner } from '~/tinycld/mail/settings/viewer-is-owner'

describe('viewerIsOwner', () => {
    it('is true when the viewer is an owner of the inbox', () => {
        expect(
            viewerIsOwner([
                { isYou: false, role: 'owner' },
                { isYou: true, role: 'owner' },
            ])
        ).toBe(true)
    })
    it('is false for a member who is not an owner', () => {
        expect(viewerIsOwner([{ isYou: true, role: 'member' }])).toBe(false)
    })
    it('is false for an admin who is not in the inbox', () => {
        expect(viewerIsOwner([{ isYou: false, role: 'owner' }])).toBe(false)
        expect(viewerIsOwner([])).toBe(false)
    })
})
