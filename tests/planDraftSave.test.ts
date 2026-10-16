import { describe, expect, it } from 'vitest'
import { planDraftSave } from '~/tinycld/mail/hooks/planDraftSave'

describe('planDraftSave', () => {
    it('creates a new draft when none is open', () => {
        expect(
            planDraftSave({ draftId: null, draftMailboxId: null, saveMailboxId: 'mb1' })
        ).toEqual({
            messageId: undefined,
            replacesDraftId: null,
        })
    })

    it('updates the open draft in place when the mailbox is unchanged', () => {
        expect(
            planDraftSave({ draftId: 'd1', draftMailboxId: 'mb1', saveMailboxId: 'mb1' })
        ).toEqual({
            messageId: 'd1',
            replacesDraftId: null,
        })
    })

    it('saves a new draft in the new mailbox and replaces the old one when From moved mailbox', () => {
        expect(
            planDraftSave({ draftId: 'd1', draftMailboxId: 'mb1', saveMailboxId: 'mb2' })
        ).toEqual({
            messageId: undefined,
            replacesDraftId: 'd1',
        })
    })
})
