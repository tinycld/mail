// @vitest-environment happy-dom
import { act, cleanup, fireEvent, render, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { AttachmentRibbon } from '~/tinycld/mail/components/AttachmentRibbon'
import { useComposeAttachments } from '~/tinycld/mail/hooks/useComposeAttachments'

// No vitest globals in this workspace, so testing-library's automatic
// afterEach cleanup never registers.
afterEach(cleanup)

const draft = (attachments: string[], id = 'd1') => ({ id, attachments })

function localFile(name: string) {
    return new File(['x'.repeat(2048)], name, { type: 'text/plain' })
}

describe('useComposeAttachments', () => {
    it("shows a resumed draft's saved files under their own names", () => {
        const { result } = renderHook(() => useComposeAttachments(draft(['hippo_ab12cd34ef.jpg'])))
        expect(result.current.items).toEqual([
            { id: 'hippo_ab12cd34ef.jpg', name: 'hippo.jpg', size: null, isSaved: true },
        ])
    })

    it('lists saved files first, then the files attached in this compose', () => {
        const { result } = renderHook(() => useComposeAttachments(draft(['hippo_ab12cd34ef.jpg'])))
        act(() => result.current.addFilesSafely([localFile('notes.txt')]))
        expect(result.current.items.map(i => i.name)).toEqual(['hippo.jpg', 'notes.txt'])
        expect(result.current.items[1]).toMatchObject({ size: 2048, isSaved: false })
        expect(result.current.files.map(f => f.name)).toEqual(['notes.txt'])
    })

    it('removes a saved file from the ribbon and reports it for the next save', () => {
        const { result } = renderHook(() =>
            useComposeAttachments(draft(['hippo_ab12cd34ef.jpg', 'notes_0123456789.txt']))
        )
        act(() => result.current.removeItem(result.current.items[0]))
        expect(result.current.items.map(i => i.name)).toEqual(['notes.txt'])
        expect(result.current.removedSaved).toEqual(['hippo_ab12cd34ef.jpg'])
    })

    it('forgets a removal once the saved draft no longer has the file', () => {
        const { result, rerender } = renderHook(
            ({ attachments }: { attachments: string[] }) =>
                useComposeAttachments(draft(attachments)),
            { initialProps: { attachments: ['hippo_ab12cd34ef.jpg'] } }
        )
        act(() => result.current.removeItem(result.current.items[0]))
        rerender({ attachments: [] })
        expect(result.current.removedSaved).toEqual([])
        expect(result.current.items).toEqual([])
    })

    it("does not carry one draft's removals to another draft", () => {
        const { result, rerender } = renderHook(
            ({ id }: { id: string }) => useComposeAttachments(draft(['hippo_ab12cd34ef.jpg'], id)),
            { initialProps: { id: 'd1' } }
        )
        act(() => result.current.removeItem(result.current.items[0]))
        rerender({ id: 'd2' })
        expect(result.current.removedSaved).toEqual([])
        expect(result.current.items.map(i => i.name)).toEqual(['hippo.jpg'])
    })

    it('clearing (a discard or a close) restores the removed saved files', () => {
        const { result } = renderHook(() => useComposeAttachments(draft(['hippo_ab12cd34ef.jpg'])))
        act(() => result.current.addFilesSafely([localFile('notes.txt')]))
        act(() => result.current.removeItem(result.current.items[0]))
        act(() => result.current.clearAll())
        expect(result.current.removedSaved).toEqual([])
        expect(result.current.items.map(i => i.name)).toEqual(['hippo.jpg'])
    })

    it('has no saved files without a draft', () => {
        const { result } = renderHook(() => useComposeAttachments(null))
        expect(result.current.items).toEqual([])
        expect(result.current.removedSaved).toEqual([])
    })
})

describe('AttachmentRibbon with a saved draft file', () => {
    it('shows the name with no size, and its remove button reports the saved file', () => {
        const removed: string[] = []
        const items = [{ id: 'hippo_ab12cd34ef.jpg', name: 'hippo.jpg', size: null, isSaved: true }]
        const view = render(
            <AttachmentRibbon isVisible items={items} onRemove={item => removed.push(item.id)} />
        )
        expect(view.getByText('hippo.jpg')).toBeTruthy()
        expect(view.queryByText(/KB|bytes/)).toBeNull()
        const removeButton = view.container.querySelector('[accessibilitylabel="Remove hippo.jpg"]')
        if (!removeButton) throw new Error('no remove button for hippo.jpg')
        fireEvent.click(removeButton)
        expect(removed).toEqual(['hippo_ab12cd34ef.jpg'])
    })
})
