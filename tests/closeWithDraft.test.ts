import { describe, expect, it, vi } from 'vitest'
import { closeWithDraft } from '~/tinycld/mail/hooks/closeWithDraft'

describe('closeWithDraft', () => {
    it('clears and closes compose only after the save succeeds', async () => {
        const finish = vi.fn()
        let resolveSave: () => void = () => {}
        const save = () =>
            new Promise<void>(resolve => {
                resolveSave = resolve
            })
        const closing = closeWithDraft({ save, finish })
        expect(finish).not.toHaveBeenCalled()
        resolveSave()
        await expect(closing).resolves.toBe(true)
        expect(finish).toHaveBeenCalledTimes(1)
    })

    it('leaves compose open with its content when the save fails', async () => {
        const finish = vi.fn()
        const save = () => Promise.reject(new Error('rule denied the write'))
        await expect(closeWithDraft({ save, finish })).resolves.toBe(false)
        expect(finish).not.toHaveBeenCalled()
    })
})
