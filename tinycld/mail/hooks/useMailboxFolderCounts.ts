import { eq } from '@tanstack/db'
import { useMemo } from 'react'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import { computeMailboxFolderCounts } from './computeMailboxFolderCounts'

export { computeMailboxFolderCounts }
export type { FolderCounts } from './computeMailboxFolderCounts'

export function useMailboxFolderCounts() {
    const [threadStateCollection, threadsCollection] = useStore('mail_thread_state', 'mail_threads')

    const { data: threadStates } = useOrgLiveQuery((query, { userOrgId }) =>
        query
            .from({ mail_thread_state: threadStateCollection })
            .where(({ mail_thread_state }) => eq(mail_thread_state.user_org, userOrgId))
    )

    const { data: threads } = useOrgLiveQuery((query) =>
        query.from({ mail_threads: threadsCollection })
    )

    return useMemo(
        () => computeMailboxFolderCounts(threadStates ?? [], threads ?? []),
        [threadStates, threads]
    )
}
