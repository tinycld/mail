import type { MailMailboxes } from '../types'

export interface MailboxesResult {
    personal: MailMailboxes | null
    shared: MailMailboxes[]
}

/**
 * Pure: given the mailboxes the current user is a member of (membership is
 * resolved by the caller's joined query), return the personal mailbox and the
 * shared ones, one entry per mailbox. Shared mailboxes are sorted by `created` ascending.
 */
export function splitMailboxes(mine: MailMailboxes[]): MailboxesResult {
    // A person can hold a direct row and group-derived rows for one mailbox.
    const unique = [...new Map(mine.map(mb => [mb.id, mb])).values()]
    const personal = unique.find(mb => mb.type === 'personal') ?? null
    const shared = unique
        .filter(mb => mb.type === 'shared')
        .sort((a, b) => a.created.localeCompare(b.created))
    return { personal, shared }
}
