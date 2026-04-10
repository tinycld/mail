import type { Transaction } from '@tanstack/react-db'
import { captureException } from '~/lib/errors'
import { useMutation } from '~/lib/mutations'
import type { MailThreadState } from '../types'

interface ThreadStateCollection {
    update(
        id: string | number,
        callback: (draft: MailThreadState) => void
    ): Transaction<Record<string, unknown>>
}

export function useThreadActions(
    // biome-ignore lint/suspicious/noExplicitAny: pbtsdb collection type is deeply generic; narrowing it is impractical
    threadStateCollection: any,
    threadState: MailThreadState | undefined,
    onNavigateBack: () => void
) {
    const col: ThreadStateCollection = threadStateCollection

    const onError = (error: unknown) => {
        captureException('Thread action failed', error)
    }

    const archiveThread = useMutation({
        mutationFn: function* () {
            if (!threadState) return
            yield col.update(threadState.id, draft => {
                draft.folder = 'archive'
            })
        },
        onSuccess: onNavigateBack,
        onError,
    })

    const spamThread = useMutation({
        mutationFn: function* () {
            if (!threadState) return
            yield col.update(threadState.id, draft => {
                draft.folder = 'spam'
            })
        },
        onSuccess: onNavigateBack,
        onError,
    })

    const trashThread = useMutation({
        mutationFn: function* () {
            if (!threadState) return
            yield col.update(threadState.id, draft => {
                draft.folder = 'trash'
            })
        },
        onSuccess: onNavigateBack,
        onError,
    })

    const moveThread = useMutation({
        mutationFn: function* (folder: MailThreadState['folder']) {
            if (!threadState) return
            yield col.update(threadState.id, draft => {
                draft.folder = folder
            })
        },
        onSuccess: onNavigateBack,
        onError,
    })

    const toggleRead = useMutation({
        mutationFn: function* () {
            if (!threadState) return
            yield col.update(threadState.id, draft => {
                draft.is_read = !draft.is_read
            })
        },
        onError,
    })

    const toggleStar = useMutation({
        mutationFn: function* () {
            if (!threadState) return
            yield col.update(threadState.id, draft => {
                draft.is_starred = !draft.is_starred
            })
        },
        onError,
    })

    const updateLabel = useMutation({
        mutationFn: function* ({ labelId, add }: { labelId: string; add: boolean }) {
            if (!threadState) return
            yield col.update(threadState.id, draft => {
                const current: string[] = draft.labels ?? []
                if (add) {
                    if (!current.includes(labelId)) {
                        draft.labels = [...current, labelId]
                    }
                } else {
                    draft.labels = current.filter(lid => lid !== labelId)
                }
            })
        },
        onError,
    })

    return {
        archiveThread,
        spamThread,
        trashThread,
        moveThread,
        toggleRead,
        toggleStar,
        updateLabel,
    }
}
