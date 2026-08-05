import { beforeEach, describe, expect, it } from 'vitest'
import {
    canAutoOpenStrip,
    MIN_HEIGHT_FOR_AUTO_OPEN,
} from '~/tinycld/mail/components/attachment-strip-auto-open'
import { useAttachmentStripStore } from '~/tinycld/mail/stores/attachment-strip-store'

beforeEach(() => {
    useAttachmentStripStore.setState({ threadId: null, expanded: false, userToggled: false })
})

describe('canAutoOpenStrip', () => {
    it('allows auto-open on a tall untouched viewport', () => {
        expect(canAutoOpenStrip(900, false)).toBe(true)
    })

    // The landscape-iPhone case: the thread fits, so isAtBottom flips true
    // right after layout and would otherwise open a 280px panel over a
    // ~400px viewport.
    it('blocks auto-open on a short viewport', () => {
        expect(canAutoOpenStrip(400, false)).toBe(false)
    })

    it('blocks auto-open once the user has toggled, however tall the viewport', () => {
        expect(canAutoOpenStrip(900, true)).toBe(false)
    })

    it('treats the threshold as inclusive', () => {
        expect(canAutoOpenStrip(MIN_HEIGHT_FOR_AUTO_OPEN, false)).toBe(true)
        expect(canAutoOpenStrip(MIN_HEIGHT_FOR_AUTO_OPEN - 1, false)).toBe(false)
    })
})

// Replays the two reported sequences against the real store, driving it the
// way the component's effect does: auto-open goes through expand(), the
// header press through toggle().
describe('auto-open gate against the strip store', () => {
    const autoOpen = (windowHeight: number) => {
        const { userToggled, expanded, expand } = useAttachmentStripStore.getState()
        if (canAutoOpenStrip(windowHeight, userToggled) && !expanded) expand()
    }

    it('leaves the strip closed on a short screen even at the bottom of the thread', () => {
        autoOpen(400)
        expect(useAttachmentStripStore.getState().expanded).toBe(false)
    })

    it('re-opening never happens after a manual collapse on a tall screen', () => {
        autoOpen(900)
        expect(useAttachmentStripStore.getState().expanded).toBe(true)

        useAttachmentStripStore.getState().toggle()
        expect(useAttachmentStripStore.getState().expanded).toBe(false)

        // Every subsequent scroll to the bottom re-fires the effect.
        autoOpen(900)
        autoOpen(900)
        expect(useAttachmentStripStore.getState().expanded).toBe(false)
    })

    it('a manual open on a short screen sticks and is not auto-collapsed', () => {
        useAttachmentStripStore.getState().toggle()
        autoOpen(400)
        expect(useAttachmentStripStore.getState().expanded).toBe(true)
    })

    // The reply form reclaims space via collapse(), which must not count as a
    // user decision — but it also must not leave the strip primed to pop back
    // open on the next scroll.
    it('reply collapse leaves the strip closed without arming a re-open', () => {
        autoOpen(900)
        useAttachmentStripStore.getState().collapse()
        expect(useAttachmentStripStore.getState().userToggled).toBe(false)
        expect(useAttachmentStripStore.getState().expanded).toBe(false)
    })

    it('switching threads restores auto-open on a tall screen', () => {
        useAttachmentStripStore.getState().resetForThread('a')
        useAttachmentStripStore.getState().toggle()
        useAttachmentStripStore.getState().resetForThread('b')

        autoOpen(900)
        expect(useAttachmentStripStore.getState().expanded).toBe(true)
    })
})
