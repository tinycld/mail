import type { Transaction } from '@tanstack/react-db'
import { useMutation } from '~/lib/mutations'
import type { ThreadListItem } from '../components/thread-list-item'
import type { MailThreadState } from '../types'

interface ThreadStateCollection {
    update(
        id: string | number,
        callback: (draft: MailThreadState) => void
    ): Transaction<Record<string, unknown>>
}

export function useMailBulkActions(
    // biome-ignore lint/suspicious/noExplicitAny: pbtsdb collection type is deeply generic; narrowing it is impractical
    threadStateCollection: any,
    selectedItems: ThreadListItem[],
    clearSelection: () => void
) {
    const col: ThreadStateCollection = threadStateCollection

    const archiveSelected = useMutation({
        mutationFn: function* () {
            yield selectedItems.map(item =>
                col.update(item.stateId, draft => {
                    draft.folder = 'archive'
                })
            )
        },
        onSuccess: clearSelection,
    })

    const spamSelected = useMutation({
        mutationFn: function* () {
            yield selectedItems.map(item =>
                col.update(item.stateId, draft => {
                    draft.folder = 'spam'
                })
            )
        },
        onSuccess: clearSelection,
    })

    const trashSelected = useMutation({
        mutationFn: function* () {
            yield selectedItems.map(item =>
                col.update(item.stateId, draft => {
                    draft.folder = 'trash'
                })
            )
        },
        onSuccess: clearSelection,
    })

    const toggleReadSelected = useMutation<void, Error, { markAsRead: boolean }>({
        mutationFn: function* ({ markAsRead }) {
            yield selectedItems.map(item =>
                col.update(item.stateId, draft => {
                    draft.is_read = markAsRead
                })
            )
        },
        onSuccess: clearSelection,
    })

    const moveSelected = useMutation<void, Error, MailThreadState['folder']>({
        mutationFn: function* (folder) {
            yield selectedItems.map(item =>
                col.update(item.stateId, draft => {
                    draft.folder = folder
                })
            )
        },
        onSuccess: clearSelection,
    })

    const toggleStarSelected = useMutation<void, Error, { star: boolean }>({
        mutationFn: function* ({ star }) {
            yield selectedItems.map(item =>
                col.update(item.stateId, draft => {
                    draft.is_starred = star
                })
            )
        },
        onSuccess: clearSelection,
    })

    const updateLabelsSelected = useMutation<void, Error, { labelId: string; add: boolean }>({
        mutationFn: function* ({ labelId, add }) {
            yield selectedItems.map(item =>
                col.update(item.stateId, draft => {
                    const labels: string[] = draft.labels ?? []
                    if (add) {
                        if (!labels.includes(labelId)) {
                            draft.labels = [...labels, labelId]
                        }
                    } else {
                        draft.labels = labels.filter(id => id !== labelId)
                    }
                })
            )
        },
        onSuccess: clearSelection,
    })

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
