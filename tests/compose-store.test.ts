import { afterEach, describe, expect, it } from 'vitest'
import { useComposeStore } from '~/tinycld/mail/stores/compose-store'

afterEach(() => useComposeStore.getState().close())

const reply = {
    messageId: 'm1',
    threadId: 't1',
    subject: 'Plan',
    to: [],
    mailboxId: 'mb_thread',
    sentToAddresses: [],
    from: { mailboxId: 'mb_support', addressId: 'help' },
}

describe('compose store', () => {
    it('openReply takes the From the reply context chose', () => {
        useComposeStore.getState().openReply(reply)
        const s = useComposeStore.getState()
        expect(s.mailboxId).toBe('mb_support')
        expect(s.fromAddressId).toBe('help')
    })

    it("openReply with no From mailbox uses the thread's mailbox", () => {
        useComposeStore.getState().openReply({ ...reply, from: { mailboxId: '', addressId: null } })
        expect(useComposeStore.getState().mailboxId).toBe('mb_thread')
    })

    it('setFrom and close', () => {
        useComposeStore.getState().setFrom('mb1', 'a1')
        expect(useComposeStore.getState().fromAddressId).toBe('a1')
        useComposeStore.getState().close()
        expect(useComposeStore.getState().fromAddressId).toBeNull()
    })
})
