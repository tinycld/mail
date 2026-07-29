import type { MailDomains, MailMailboxAliases, MailMailboxes } from '../types'

export interface SendableIdentity {
    mailboxId: string
    mailboxDisplayName: string
    primaryAddress: string
    aliases: { id: string; address: string }[]
}

/**
 * One row per (membership × alias) from useSendableIdentities' joined query.
 * `alias` is undefined for a mailbox with no aliases (left join).
 */
export interface SendableIdentityRow {
    mailbox: MailMailboxes
    domain: MailDomains
    alias?: MailMailboxAliases
}

/**
 * Pure: group the joined rows into one identity per mailbox, personal first,
 * then shared by `created` ascending — the order pickers present them in.
 */
export function groupSendableIdentities(rows: SendableIdentityRow[]): SendableIdentity[] {
    const byMailbox = new Map<
        string,
        { mailbox: MailMailboxes; domainName: string; aliases: { id: string; address: string }[] }
    >()
    for (const row of rows) {
        let entry = byMailbox.get(row.mailbox.id)
        if (!entry) {
            entry = { mailbox: row.mailbox, domainName: row.domain.domain, aliases: [] }
            byMailbox.set(row.mailbox.id, entry)
        }
        if (row.alias) {
            entry.aliases.push({
                id: row.alias.id,
                address: `${row.alias.address}@${entry.domainName}`,
            })
        }
    }

    const entries = Array.from(byMailbox.values())
    const personal = entries.filter(e => e.mailbox.type === 'personal')
    const shared = entries
        .filter(e => e.mailbox.type === 'shared')
        .sort((a, b) => a.mailbox.created.localeCompare(b.mailbox.created))

    return [...personal, ...shared].map(e => ({
        mailboxId: e.mailbox.id,
        mailboxDisplayName: e.mailbox.display_name || e.mailbox.address,
        primaryAddress: `${e.mailbox.address}@${e.domainName}`,
        aliases: e.aliases,
    }))
}
