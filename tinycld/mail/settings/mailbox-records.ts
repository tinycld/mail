import type { MailboxFormValues } from './MailboxForm'

export type MailboxType = 'personal' | 'shared'

// The rows a created mailbox writes: the mailbox, and its creator as owner.
// A personal mailbox is the creator's own address, the same shape the server
// gives a user who joins after a domain is verified.
export function newMailboxRecords({
    data,
    type,
    userId,
    mailboxId,
    memberId,
}: {
    data: MailboxFormValues
    type: MailboxType
    userId: string
    mailboxId: string
    memberId: string
}) {
    return {
        mailbox: {
            id: mailboxId,
            address: data.address,
            domain: data.domain,
            display_name: data.display_name,
            name: data.display_name,
            type,
        },
        member: { id: memberId, mailbox: mailboxId, user: userId, role: 'owner' as const },
    }
}

// The form's address rule allows only these characters, so a username is
// reduced to them before it is offered as the default address.
export function defaultAddressFor(username: string): string {
    return username
        .trim()
        .toLowerCase()
        .replace(/[^a-z0-9._-]/g, '')
        .slice(0, 64)
}
