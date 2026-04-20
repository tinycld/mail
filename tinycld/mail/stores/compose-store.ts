import { create } from '@tinycld/core/lib/store'
import type { ComposeMode, DraftContext, ReplyContext } from '../hooks/useComposeState'

interface ComposeStoreState {
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

export const useComposeStore = create<ComposeStoreState>((set) => ({
    mode: 'closed',
    replyContext: null,
    draftContext: null,
    open: () => set({ mode: 'open' }),
    minimize: () => set({ mode: 'minimized' }),
    maximize: () => set({ mode: 'maximized' }),
    close: () => set({ mode: 'closed', replyContext: null, draftContext: null }),
    openReply: (context) => set({ replyContext: context, mode: 'inline' }),
    openDraft: (context) => set({ draftContext: context, replyContext: null, mode: 'open' }),
}))
