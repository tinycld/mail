import { useCallback, useState } from 'react'
import { cleanFilename } from '../components/attachment-preview-source'
import { type AttachmentFile, useAttachments } from './useAttachments'

/** One chip in the compose ribbon: a draft's saved file or a newly attached one. */
export interface RibbonItem {
    id: string
    name: string
    /** Bytes; null for a saved file, whose size the draft row does not carry. */
    size: number | null
    isSaved: boolean
}

interface SavedDraft {
    id: string
    attachments: string[]
}

/** Pure: the ribbon chips, the draft's saved files first. */
export function composeRibbonItems(saved: string[], local: AttachmentFile[]): RibbonItem[] {
    return [
        ...saved.map(name => ({ id: name, name: cleanFilename(name), size: null, isSaved: true })),
        ...local.map(a => ({ id: a.id, name: a.name, size: a.size, isSaved: false })),
    ]
}

/**
 * The saved files of the open draft, less the ones the sender removed. The
 * saved files come from the draft row, so they follow it live; only the
 * removals are local. A removal is keyed to its draft, so it cannot leak to
 * the next draft opened, and it drops out once the draft row no longer has
 * the file (the save that removed it succeeded).
 */
function useSavedAttachments(draft: SavedDraft | null) {
    const draftId = draft?.id ?? ''
    const onDraft = draft?.attachments ?? []
    const [removal, setRemoval] = useState({ draftId: '', names: [] as string[] })
    const removed =
        removal.draftId === draftId ? removal.names.filter(name => onDraft.includes(name)) : []
    const kept = onDraft.filter(name => !removed.includes(name))

    const remove = useCallback(
        (name: string) =>
            setRemoval(prev => ({
                draftId,
                names: [...(prev.draftId === draftId ? prev.names : []), name],
            })),
        [draftId]
    )
    const reset = useCallback(() => setRemoval({ draftId: '', names: [] }), [])

    return { kept, removed, remove, reset }
}

/** The compose window's attachments: the open draft's saved files plus new ones. */
export function useComposeAttachments(draft: SavedDraft | null) {
    const local = useAttachments()
    const saved = useSavedAttachments(draft)
    const { removeFile, clearAll: clearLocal } = local
    const { remove: removeSaved, reset: resetSaved } = saved

    const removeItem = useCallback(
        (item: RibbonItem) => (item.isSaved ? removeSaved(item.id) : removeFile(item.id)),
        [removeSaved, removeFile]
    )
    const clearAll = useCallback(() => {
        clearLocal()
        resetSaved()
    }, [clearLocal, resetSaved])

    return {
        items: composeRibbonItems(saved.kept, local.attachments),
        files: local.attachments.map(a => a.file),
        removedSaved: saved.removed,
        addFilesSafely: local.addFilesSafely,
        removeItem,
        clearAll,
    }
}
