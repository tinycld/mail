import { useComposeStore } from '../stores/compose-store'
import type { FromSelection } from './defaultFromIdentity'

export type ComposeMode = 'closed' | 'minimized' | 'open' | 'maximized' | 'inline'

export interface ReplyContext {
    messageId: string
    threadId: string
    subject: string
    to: { name: string; email: string }[]
    mailboxId: string
    sentToAddresses: string[]
    from: FromSelection
}

export interface DraftContext {
    threadId: string
    mailboxId: string
}

export interface ComposeState {
    mode: ComposeMode
    replyContext: ReplyContext | null
    draftContext: DraftContext | null
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

export function useCompose() {
    return useComposeStore()
}
