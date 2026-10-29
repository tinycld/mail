export type MailboxType = 'personal' | 'shared'

// The rows a created mailbox writes: the mailbox, its creator as owner, and an
// added address for each domain after the first. The first domain is the
// mailbox's own address; the server writes its primary address row. A
// personal mailbox is the creator's own address, the same shape the server
// gives a user who joins after a domain is verified.
export function newMailboxRecords({
    address,
    displayName,
    domains,
    type,
    userId,
    newId,
}: {
    address: string
    displayName: string
    domains: readonly string[]
    type: MailboxType
    userId: string
    newId: () => string
}) {
    const [primaryDomain = '', ...otherDomains] = domains
    const mailboxId = newId()
    return {
        mailbox: {
            id: mailboxId,
            address,
            domain: primaryDomain,
            display_name: displayName,
            name: displayName,
            type,
        },
        member: {
            id: newId(),
            mailbox: mailboxId,
            user: userId,
            group: '',
            role: 'owner' as const,
        },
        addresses: otherDomains.map(domain => ({
            id: newId(),
            local_part: address,
            domain,
            mailbox: mailboxId,
            source: 'manual' as const,
            is_primary: false,
            via_group: '',
            group: '',
            created: '',
            updated: '',
        })),
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
