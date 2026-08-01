import type { MailMailboxes } from '../types'

export interface MailboxesResult {
    personal: MailMailboxes | null
    shared: MailMailboxes[]
}

/**
 * Pure: given the mailboxes the current user is a member of (membership is
 * resolved by the caller's joined query), return the personal mailbox and the
 * shared ones. Shared mailboxes are sorted by `created` ascending.
 */
export function splitMailboxes(mine: MailMailboxes[]): MailboxesResult {
    const personal = mine.find(mb => mb.type === 'personal') ?? null
    const shared = mine
        .filter(mb => mb.type === 'shared')
        .sort((a, b) => a.created.localeCompare(b.created))
    return { personal, shared }
}
