import { create } from '@tinycld/core/lib/store'
import type { ComposeMode, DraftContext, ReplyContext } from '../hooks/useComposeState'

interface ComposeStoreState {
    mode: ComposeMode
    replyContext: ReplyContext | null
    draftContext: DraftContext | null
    // The mailbox the message is sent from, and the address in it. An empty
    // mailboxId means the user's personal mailbox; a null fromAddressId means
    // that mailbox's default address.
    mailboxId: string
    fromAddressId: string | null
    open: () => void
    minimize: () => void
    maximize: () => void
    close: () => void
    openReply: (context: ReplyContext) => void
    openDraft: (context: DraftContext) => void
    setFrom: (mailboxId: string, addressId: string | null) => void
}

export const useComposeStore = create<ComposeStoreState>(set => ({
    mode: 'closed',
    replyContext: null,
    draftContext: null,
    mailboxId: '',
    fromAddressId: null,
    open: () => set({ mode: 'open' }),
    minimize: () => set({ mode: 'minimized' }),
    maximize: () => set({ mode: 'maximized' }),
    close: () =>
        set({
            mode: 'closed',
            replyContext: null,
            draftContext: null,
            mailboxId: '',
            fromAddressId: null,
        }),
    openReply: context =>
        set({
            replyContext: context,
            mode: 'inline',
            mailboxId: context.from.mailboxId || context.mailboxId,
            fromAddressId: context.from.addressId,
        }),
    openDraft: context =>
        set({
            draftContext: context,
            replyContext: null,
            mode: 'open',
            mailboxId: context.mailboxId,
            fromAddressId: null,
        }),
    setFrom: (mailboxId, addressId) => set({ mailboxId, fromAddressId: addressId }),
}))
