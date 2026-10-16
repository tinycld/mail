interface CloseWithDraftParams {
    save: () => Promise<unknown>
    // Clears the editor and form and closes compose.
    finish: () => void
}

/**
 * Closes compose only after its draft is saved. A failed save leaves compose
 * open with its content; useSaveDraft has already told the user why (a toast
 * online, onOffline offline). Returns whether compose closed.
 */
export async function closeWithDraft({ save, finish }: CloseWithDraftParams): Promise<boolean> {
    try {
        await save()
    } catch {
        return false
    }
    finish()
    return true
}
