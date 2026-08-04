import { beforeEach, describe, expect, it } from 'vitest'
import { useAttachmentStripStore } from '~/tinycld/mail/stores/attachment-strip-store'

function reset() {
    useAttachmentStripStore.setState({ threadId: null, expanded: false, userToggled: false })
}

describe('attachment-strip-store', () => {
    beforeEach(reset)

    it('expand() sets expanded to true', () => {
        useAttachmentStripStore.getState().expand()
        expect(useAttachmentStripStore.getState().expanded).toBe(true)
    })

    it('collapse() sets expanded to false', () => {
        useAttachmentStripStore.getState().expand()
        useAttachmentStripStore.getState().collapse()
        expect(useAttachmentStripStore.getState().expanded).toBe(false)
    })

    it('toggle() flips expanded', () => {
        const { toggle } = useAttachmentStripStore.getState()
        toggle()
        expect(useAttachmentStripStore.getState().expanded).toBe(true)
        toggle()
        expect(useAttachmentStripStore.getState().expanded).toBe(false)
    })

    it('resetForThread sets threadId and expanded=false on first call', () => {
        useAttachmentStripStore.getState().resetForThread('a')
        expect(useAttachmentStripStore.getState().threadId).toBe('a')
        expect(useAttachmentStripStore.getState().expanded).toBe(false)
    })

    it('resetForThread to a NEW thread resets expanded', () => {
        useAttachmentStripStore.getState().resetForThread('a')
        useAttachmentStripStore.getState().expand()
        useAttachmentStripStore.getState().resetForThread('b')
        expect(useAttachmentStripStore.getState().threadId).toBe('b')
        expect(useAttachmentStripStore.getState().expanded).toBe(false)
    })

    it('resetForThread to the SAME thread is a no-op', () => {
        useAttachmentStripStore.getState().resetForThread('a')
        useAttachmentStripStore.getState().expand()
        useAttachmentStripStore.getState().resetForThread('a')
        expect(useAttachmentStripStore.getState().threadId).toBe('a')
        expect(useAttachmentStripStore.getState().expanded).toBe(true)
    })

    // userToggled switches off the scroll-driven auto-open. Only a header
    // press may set it — if expand()/collapse() set it too, the auto-open
    // path would immediately disarm itself and the reply form's collapse
    // would permanently suppress a behavior the user never opted out of.
    it('toggle() marks the strip as user-toggled', () => {
        useAttachmentStripStore.getState().toggle()
        expect(useAttachmentStripStore.getState().userToggled).toBe(true)
    })

    it('toggle() to closed still leaves userToggled set', () => {
        const { toggle } = useAttachmentStripStore.getState()
        toggle()
        toggle()
        expect(useAttachmentStripStore.getState().expanded).toBe(false)
        expect(useAttachmentStripStore.getState().userToggled).toBe(true)
    })

    it('expand() does not mark the strip as user-toggled', () => {
        useAttachmentStripStore.getState().expand()
        expect(useAttachmentStripStore.getState().userToggled).toBe(false)
    })

    it('collapse() does not mark the strip as user-toggled', () => {
        useAttachmentStripStore.getState().collapse()
        expect(useAttachmentStripStore.getState().userToggled).toBe(false)
    })

    it('resetForThread to a NEW thread clears userToggled', () => {
        useAttachmentStripStore.getState().resetForThread('a')
        useAttachmentStripStore.getState().toggle()
        useAttachmentStripStore.getState().resetForThread('b')
        expect(useAttachmentStripStore.getState().userToggled).toBe(false)
    })

    it('resetForThread to the SAME thread preserves userToggled', () => {
        useAttachmentStripStore.getState().resetForThread('a')
        useAttachmentStripStore.getState().toggle()
        useAttachmentStripStore.getState().resetForThread('a')
        expect(useAttachmentStripStore.getState().userToggled).toBe(true)
    })
})
