import type { SendableAddress, SendableIdentity } from './groupSendableIdentities'
import type { MailSendBlocker, MailSendReadiness } from './useMailSendReadiness'

export interface ResolvedFrom {
    identity: SendableIdentity
    address: SendableAddress
}

/**
 * Pure: the mailbox and address a compose sends from. The From row shows
 * exactly this and the send / draft payload uses exactly this, so the two
 * cannot disagree.
 *
 * The wanted mailbox is the picked one, else `fallbackMailboxId`, else the
 * first identity (the personal mailbox sorts first). When the wanted mailbox
 * is not among the identities — still loading, or it has no sendable
 * address — this returns null: the From row shows nothing and Send stays
 * disabled. It never substitutes another mailbox, which would show one From
 * and send from another.
 *
 * A picked address that is not in the mailbox (deleted since, or stale)
 * resolves to that mailbox's default address.
 */
export function resolveComposeFrom(
    identities: SendableIdentity[],
    storeMailboxId: string,
    storeAddressId: string | null,
    fallbackMailboxId: string
): ResolvedFrom | null {
    const wantedId = storeMailboxId || fallbackMailboxId
    const identity = wantedId ? identities.find(i => i.mailboxId === wantedId) : identities[0]
    if (!identity) return null
    const address =
        identity.addresses.find(a => a.id === storeAddressId) ??
        identity.addresses.find(a => a.id === identity.defaultAddressId) ??
        identity.addresses[0]
    if (!address) return null
    return { identity, address }
}

export interface ComposeSendBlocker {
    blocker: MailSendBlocker | null
    message: string | null
}

function domainOf(email: string) {
    return email.slice(email.lastIndexOf('@') + 1)
}

/**
 * Pure: what blocks or warns about a send from the chosen From. The domain
 * check uses the chosen address, not the personal mailbox's domain. With no
 * resolved address, only the readiness blockers that mean "nothing to send
 * from" apply (no mailbox, no domain); its personal-domain warning would be
 * about a mailbox the user may not be sending from.
 */
export function composeSendBlocker(
    from: ResolvedFrom | null,
    readiness: MailSendReadiness
): ComposeSendBlocker {
    if (from) {
        if (from.address.domainVerified) return { blocker: null, message: null }
        return {
            blocker: 'domain-unverified',
            message: `The domain "${domainOf(from.address.email)}" is not verified. Outgoing mail will fail until an admin completes verification in Settings → Mail.`,
        }
    }
    if (readiness.blocker === 'no-mailbox' || readiness.blocker === 'no-domain') {
        return { blocker: readiness.blocker, message: readiness.message }
    }
    return { blocker: null, message: null }
}
