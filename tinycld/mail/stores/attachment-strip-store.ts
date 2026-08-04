import { create } from '@tinycld/core/lib/store'

interface AttachmentStripState {
    threadId: string | null
    expanded: boolean
    /**
     * Set once the user opens or closes the strip themselves, which switches
     * off the scroll-driven auto-open for the rest of the thread. Only
     * `toggle()` (the header press) counts — `expand()`/`collapse()` are
     * programmatic, so the auto-open path and the reply form's
     * space-reclaiming collapse don't masquerade as a user decision.
     */
    userToggled: boolean
    expand: () => void
    collapse: () => void
    toggle: () => void
    resetForThread: (threadId: string) => void
}

export const useAttachmentStripStore = create<AttachmentStripState>((set, get) => ({
    threadId: null,
    expanded: false,
    userToggled: false,
    expand: () => set({ expanded: true }),
    collapse: () => set({ expanded: false }),
    toggle: () => set(state => ({ expanded: !state.expanded, userToggled: true })),
    resetForThread: threadId => {
        if (get().threadId === threadId) return
        set({ threadId, expanded: false, userToggled: false })
    },
}))
