interface PlanDraftSaveParams {
    // The open draft's message id, if compose was opened on a draft.
    draftId: string | null
    // The mailbox that draft lives in.
    draftMailboxId: string | null
    // The mailbox the draft is saved into now (the resolved From).
    saveMailboxId: string
}

export interface DraftSavePlan {
    // Passed as message_id: update this draft in place.
    messageId: string | undefined
    // A draft to delete once the new one is saved.
    replacesDraftId: string | null
}

/**
 * Pure: how a draft save treats the open draft. The server refuses to update
 * a draft into a different mailbox ("Draft does not belong to this
 * mailbox"), so when the user changed From to another mailbox the draft is
 * saved as a new one there, and the old one is deleted after that succeeds.
 */
export function planDraftSave({
    draftId,
    draftMailboxId,
    saveMailboxId,
}: PlanDraftSaveParams): DraftSavePlan {
    if (!draftId) return { messageId: undefined, replacesDraftId: null }
    if (draftMailboxId && draftMailboxId !== saveMailboxId) {
        return { messageId: undefined, replacesDraftId: draftId }
    }
    return { messageId: draftId, replacesDraftId: null }
}
