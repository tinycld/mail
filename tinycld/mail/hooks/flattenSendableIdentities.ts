import type { MailDomains, MailMailboxAliases, MailMailboxes } from '../types'

export interface SendableIdentity {
    mailboxId: string
    mailboxDisplayName: string
    primaryAddress: string
    aliases: { id: string; address: string }[]
}

export function flattenSendableIdentities(
    mailboxes: MailMailboxes[],
    allAliases: MailMailboxAliases[],
    allDomains: MailDomains[]
): SendableIdentity[] {
    const domainMap = new Map(allDomains.map((d) => [d.id, d.domain]))
    const aliasesByMailbox = new Map<string, { id: string; address: string }[]>()
    for (const a of allAliases) {
        const list = aliasesByMailbox.get(a.mailbox) ?? []
        list.push({ id: a.id, address: a.address })
        aliasesByMailbox.set(a.mailbox, list)
    }

    return mailboxes.map((mb) => {
        const domainName = domainMap.get(mb.domain) ?? ''
        const aliases = (aliasesByMailbox.get(mb.id) ?? []).map((a) => ({
            id: a.id,
            address: `${a.address}@${domainName}`,
        }))
        return {
            mailboxId: mb.id,
            mailboxDisplayName: mb.display_name || mb.address,
            primaryAddress: `${mb.address}@${domainName}`,
            aliases,
        }
    })
}
