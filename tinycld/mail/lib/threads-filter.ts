// Build a PocketBase filter expression for the paginated mail_threads query.
// Uses back-relation syntax (mail_thread_state_via_thread.<field>) so the
// server joins state and threads itself — no need to pre-fetch thread ids.
export function buildThreadsFilter(params: {
    mailboxIds: string[]
    userIds: string[]
    folder: string | null
}): string {
    const clauses: string[] = []

    if (params.mailboxIds.length === 1) {
        clauses.push(`mailbox = ${quote(params.mailboxIds[0])}`)
    } else {
        clauses.push(`(${params.mailboxIds.map(id => `mailbox = ${quote(id)}`).join(' || ')})`)
    }

    // Each thread must have a thread_state row owned by one of the relevant
    // users (just the current user normally; widened to co-members on shared
    // mailbox sent/drafts views).
    if (params.userIds.length === 1) {
        clauses.push(`mail_thread_state_via_thread.user ?= ${quote(params.userIds[0])}`)
    } else {
        clauses.push(
            `(${params.userIds.map(id => `mail_thread_state_via_thread.user ?= ${quote(id)}`).join(' || ')})`
        )
    }

    // Folder semantics mirror the mail_folder_counts view:
    //   inbox    — folder='inbox' (no unread restriction; the row visibility
    //              isn't a count, the unread is a row-level visual)
    //   starred  — is_starred=true (any folder)
    //   sent     — is_sent=true, outside trash and spam (a reply leaves its
    //              thread in its folder and flags it sent)
    //   all      — every state row for the user, no folder restriction
    //   <other>  — folder=<value>
    const folder = params.folder ?? 'inbox'
    if (folder === 'starred') {
        clauses.push('mail_thread_state_via_thread.is_starred ?= true')
    } else if (folder === 'sent') {
        clauses.push('mail_thread_state_via_thread.is_sent ?= true')
        clauses.push('mail_thread_state_via_thread.folder ?!= "trash"')
        clauses.push('mail_thread_state_via_thread.folder ?!= "spam"')
    } else if (folder === 'all' || folder === 'all-inboxes') {
        // No folder restriction beyond having a state row in the right scope.
    } else {
        clauses.push(`mail_thread_state_via_thread.folder ?= ${quote(folder)}`)
    }

    return clauses.join(' && ')
}

// PocketBase filter values — same shape as pb.filter() but inline so we don't
// need an extra round-trip through the filter helper.
export function quote(s: string): string {
    return `"${s.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`
}
