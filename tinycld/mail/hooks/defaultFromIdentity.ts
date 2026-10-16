import type { SendableIdentity } from './groupSendableIdentities'

export interface FromSelection {
    mailboxId: string
    // null sends as that mailbox's default address.
    addressId: string | null
}

interface Recipient {
    name: string
    email: string
}

/**
 * Accepts either a bare email ("alice@acme.com") or a display-name-prefixed
 * address ('"Alice" <alice@acme.com>') and returns the lowercase bare address,
 * or null if not extractable.
 */
export function extractBareAddress(raw: string): string | null {
    const trimmed = raw.trim()
    if (!trimmed) return null
    const angleMatch = trimmed.match(/<([^>]+)>/)
    if (angleMatch) return angleMatch[1].trim().toLowerCase()
    return trimmed.toLowerCase()
}

function findAddress(
    identities: SendableIdentity[],
    match: (email: string, id: string) => boolean
) {
    for (const identity of identities) {
        const address = identity.addresses.find(a => match(a.email.toLowerCase(), a.id))
        if (address) return { mailboxId: identity.mailboxId, addressId: address.id }
    }
    return null
}

interface PickReplyFromParams {
    identities: SendableIdentity[]
    deliveredToAddressId: string | null
    headerAddresses: string[]
    threadMailboxId: string
}

/**
 * Pure: which address a reply or forward is sent from.
 *  1. the address the message was delivered to (mail_messages.delivered_to);
 *  2. else one of my addresses found in its To/Cc (mail stored before
 *     delivered_to existed has none), the thread mailbox's addresses first:
 *     a message to both support@ and me@ in a Support thread replies as
 *     Support;
 *  3. else the thread's mailbox, with its default address;
 *  4. else my first mailbox.
 */
export function pickReplyFrom({
    identities,
    deliveredToAddressId,
    headerAddresses,
    threadMailboxId,
}: PickReplyFromParams): FromSelection {
    if (identities.length === 0) return { mailboxId: '', addressId: null }

    const delivered = deliveredToAddressId
        ? findAddress(identities, (_, id) => id === deliveredToAddressId)
        : null
    if (delivered) return delivered

    const header = new Set(headerAddresses.map(extractBareAddress).filter(a => a !== null))
    const threadFirst = [
        ...identities.filter(i => i.mailboxId === threadMailboxId),
        ...identities.filter(i => i.mailboxId !== threadMailboxId),
    ]
    const fromHeader = findAddress(threadFirst, email => header.has(email))
    if (fromHeader) return fromHeader

    const thread = identities.find(i => i.mailboxId === threadMailboxId) ?? identities[0]
    return { mailboxId: thread.mailboxId, addressId: null }
}

interface FilterOwnAddressesParams {
    identities: SendableIdentity[]
    recipients: Recipient[]
}

/** Pure: drop every address of the given identities from a recipient list. */
export function filterOwnAddresses({
    identities,
    recipients,
}: FilterOwnAddressesParams): Recipient[] {
    const own = new Set(
        identities.flatMap(identity => identity.addresses.map(a => a.email.toLowerCase()))
    )
    return recipients.filter(r => !own.has(r.email.toLowerCase()))
}
