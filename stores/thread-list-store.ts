import { create } from '~/lib/store'

interface ThreadListStoreState {
    threadIds: string[]
    setThreadIds: (ids: string[]) => void
    /**
     * Keyboard-driven focus index into the current mail list. Persisted
     * across mount/unmount so round-tripping list → conversation → Escape
     * lands back on the row the user was on.
     */
    focusedIndex: number
    setFocusedIndex: (i: number | ((prev: number) => number)) => void
}

export const useThreadListStore = create<ThreadListStoreState>((set) => ({
    threadIds: [],
    setThreadIds: (ids) => set({ threadIds: ids }),
    focusedIndex: 0,
    setFocusedIndex: (next) =>
        set((state) => ({
            focusedIndex: typeof next === 'function' ? next(state.focusedIndex) : next,
        })),
}))

export function useThreadListContext() {
    return useThreadListStore()
}
