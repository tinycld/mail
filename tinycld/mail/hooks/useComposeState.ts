import { useComposeStore } from '../stores/compose-store'

export type ComposeMode = 'closed' | 'minimized' | 'open' | 'maximized' | 'inline'

export interface ReplyContext {
    messageId: string
    threadId: string
    subject: string
    to: { name: string; email: string }[]
}

export interface DraftContext {
    messageId: string
    threadId: string
    subject: string
    to: { name: string; email: string }[]
    cc: { name: string; email: string }[]
    bcc: { name: string; email: string }[]
    htmlBody: string
    textBody: string
}

export interface ComposeState {
    mode: ComposeMode
    replyContext: ReplyContext | null
    draftContext: DraftContext | null
    open: () => void
    minimize: () => void
    maximize: () => void
    close: () => void
    openReply: (context: ReplyContext) => void
    openDraft: (context: DraftContext) => void
}

export function useCompose() {
    return useComposeStore()
}
