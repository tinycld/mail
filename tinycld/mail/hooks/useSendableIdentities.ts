import { eq } from '@tanstack/db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useMyLiveQuery } from '@tinycld/core/lib/use-my-live-query'
import { useMemo } from 'react'
import { groupSendableIdentities } from './groupSendableIdentities'

export type { SendableAddress, SendableIdentity } from './groupSendableIdentities'

export function useSendableIdentities() {
    const [
        membersCollection,
        mailboxesCollection,
        addressesCollection,
        domainsCollection,
        identitiesCollection,
    ] = useStore(
        'mail_mailbox_members',
        'mail_mailboxes',
        'mail_addresses',
        'mail_domains',
        'mail_identities'
    )

    // One query: membership → mailbox → each address → its domain. A mailbox
    // with no address row drops out (it has nothing to send as). The identity
    // is left-joined: only a personal inbox has one, and it carries the
    // owner's chosen default.
    const { data: rows } = useMyLiveQuery(
        (query, { userId }) =>
            query
                .from({ member: membersCollection })
                .innerJoin({ mailbox: mailboxesCollection }, ({ member, mailbox }) =>
                    eq(member.mailbox, mailbox.id)
                )
                .innerJoin({ address: addressesCollection }, ({ mailbox, address }) =>
                    eq(address.mailbox, mailbox.id)
                )
                .innerJoin({ domain: domainsCollection }, ({ address, domain }) =>
                    eq(address.domain, domain.id)
                )
                .leftJoin({ identity: identitiesCollection }, ({ mailbox, identity }) =>
                    eq(identity.inbox, mailbox.id)
                )
                .where(({ member }) => eq(member.user, userId)),
        { queryKey: ['mail_sendable_identities'] }
    )

    return useMemo(() => groupSendableIdentities(rows ?? []), [rows])
}
