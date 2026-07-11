import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import type { useStore } from '@tinycld/core/lib/pocketbase'
import { useLabelMutations } from '@tinycld/core/ui/hooks/useLabelMutations'
import type { ThreadListItem } from '../components/thread-list-item'
import type { MailThreadState } from '../types'

type ThreadStateCollection = ReturnType<typeof useStore<['mail_thread_state']>>[0]

export function useMailBulkActions(
    threadStateCollection: ThreadStateCollection,
    selectedItems: ThreadListItem[],
    clearSelection: () => void
) {
    const col = threadStateCollection
    const { assignLabel, unassignLabel } = useLabelMutations()

    const archiveSelected = useMutation({
        mutationFn: mutation(function* () {
            yield selectedItems.map(item =>
                col.update(item.stateId, draft => {
                    draft.folder = 'archive'
                })
            )
        }),
        onSuccess: clearSelection,
    })

    const spamSelected = useMutation({
        mutationFn: mutation(function* () {
            yield selectedItems.map(item =>
                col.update(item.stateId, draft => {
                    draft.folder = 'spam'
                })
            )
        }),
        onSuccess: clearSelection,
    })

    const trashSelected = useMutation({
        mutationFn: mutation(function* () {
            yield selectedItems.map(item =>
                col.update(item.stateId, draft => {
                    draft.folder = 'trash'
                })
            )
        }),
        onSuccess: clearSelection,
    })

    const toggleReadSelected = useMutation<void, Error, { markAsRead: boolean }>({
        mutationFn: mutation(function* ({ markAsRead }) {
            yield selectedItems.map(item =>
                col.update(item.stateId, draft => {
                    draft.is_read = markAsRead
                })
            )
        }),
        onSuccess: clearSelection,
    })

    const moveSelected = useMutation<void, Error, MailThreadState['folder']>({
        mutationFn: mutation(function* (folder) {
            yield selectedItems.map(item =>
                col.update(item.stateId, draft => {
                    draft.folder = folder
                })
            )
        }),
        onSuccess: clearSelection,
    })

    const toggleStarSelected = useMutation<void, Error, { star: boolean }>({
        mutationFn: mutation(function* ({ star }) {
            yield selectedItems.map(item =>
                col.update(item.stateId, draft => {
                    draft.is_starred = star
                })
            )
        }),
        onSuccess: clearSelection,
    })

    const updateLabelsSelected = {
        mutate: ({ labelId, add }: { labelId: string; add: boolean }) => {
            for (const item of selectedItems) {
                if (add) {
                    assignLabel.mutate({
                        labelId,
                        recordId: item.stateId,
                        collection: 'mail_thread_state',
                    })
                } else {
                    unassignLabel.mutate({
                        labelId,
                        recordId: item.stateId,
                        collection: 'mail_thread_state',
                    })
                }
            }
            clearSelection()
        },
    }

    return {
        archiveSelected,
        spamSelected,
        trashSelected,
        toggleReadSelected,
        moveSelected,
        toggleStarSelected,
        updateLabelsSelected,
    }
}
