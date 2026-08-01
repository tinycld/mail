import { captureException, errorToString } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { notify } from '@tinycld/core/lib/notify'
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

    // A bulk action failing across N threads was entirely silent: no toast,
    // no capture, selection kept — indistinguishable from success with the
    // rows quietly reverting when the optimistic writes rolled back.
    const bulkActionFailed = (action: string) => (error: unknown) => {
        captureException(`mail.bulk.${action}`, error, { count: selectedItems.length })
        notify.emit({
            event: 'mutation.error',
            title: 'Action failed',
            body: errorToString(error),
            durationMs: 6000,
            data: { operation: `mail.bulk.${action}`, error: errorToString(error) },
        })
    }

    const archiveSelected = useMutation({
        mutationFn: mutation(function* () {
            yield selectedItems.map(item =>
                col.update(item.stateId, draft => {
                    draft.folder = 'archive'
                })
            )
        }),
        onSuccess: clearSelection,
        onError: bulkActionFailed('archive'),
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
        onError: bulkActionFailed('spam'),
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
        onError: bulkActionFailed('trash'),
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
        onError: bulkActionFailed('toggle_read'),
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
        onError: bulkActionFailed('move'),
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
        onError: bulkActionFailed('toggle_star'),
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
