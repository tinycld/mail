import { create } from '~/lib/store'

interface ThreadListStoreState {
    threadIds: string[]
    setThreadIds: (ids: string[]) => void
}

export const useThreadListStore = create<ThreadListStoreState>(set => ({
    threadIds: [],
    setThreadIds: ids => set({ threadIds: ids }),
}))

export function useThreadListContext() {
    return useThreadListStore()
}
