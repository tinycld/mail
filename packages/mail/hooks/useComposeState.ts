import { createContext, useContext } from 'react'

export type ComposeMode = 'closed' | 'minimized' | 'open' | 'maximized' | 'inline'

export interface ReplyContext {
    messageId: string
    threadId: string
    subject: string
    to: { name: string; email: string }[]
}

export interface ComposeState {
    mode: ComposeMode
    replyContext: ReplyContext | null
    open: () => void
    minimize: () => void
    maximize: () => void
    close: () => void
    openReply: (context: ReplyContext) => void
}

export const ComposeContext = createContext<ComposeState>({
    mode: 'closed',
    replyContext: null,
    open: () => {},
    minimize: () => {},
    maximize: () => {},
    close: () => {},
    openReply: () => {},
})

export function useCompose() {
    return useContext(ComposeContext)
}
