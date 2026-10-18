import { eq } from '@tanstack/db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useMyLiveQuery } from '@tinycld/core/lib/use-my-live-query'
import { materialize } from 'pbtsdb'
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

    // The person's own membership rows drive the query: they load and
    // subscribe by `user` alone, so access to a new inbox (an owner adds the
    // person, or a group grant) reaches the From list without a reload. Each
    // row reads its mailbox, its addresses with their domains, and the
    // identity (only a personal inbox has one; it carries the owner's chosen
    // default) from their own collections. Inner joins did not do this: an
    // inner join picks its active side by store size, and when the other side
    // won, memberships were subscribed only for the mailboxes already known.
    const { data: rows } = useMyLiveQuery(
        (query, { userId }) =>
            query
                .from({ member: membersCollection })
                .where(({ member }) => eq(member.user, userId))
                .select(({ member }) => ({
                    id: member.id,
                    mailbox: materialize(
                        query
                            .from({ mailbox: mailboxesCollection })
                            .where(({ mailbox }) => eq(mailbox.id, member.mailbox))
                            .findOne()
                    ),
                    addresses: materialize(
                        query
                            .from({ address: addressesCollection })
                            .where(({ address }) => eq(address.mailbox, member.mailbox))
                            .select(({ address }) => ({
                                ...address,
                                domainRecord: materialize(
                                    query
                                        .from({ domain: domainsCollection })
                                        .where(({ domain }) => eq(domain.id, address.domain))
                                        .findOne()
                                ),
                            }))
                    ),
                    identity: materialize(
                        query
                            .from({ identity: identitiesCollection })
                            .where(({ identity }) => eq(identity.inbox, member.mailbox))
                            .findOne()
                    ),
                })),
        { queryKey: ['mail_sendable_identities'] }
    )

    return useMemo(() => {
        type Row = NonNullable<typeof rows>[number]
        // One entry per address. A membership whose mailbox is gone, a mailbox
        // with no address, and an address whose domain is gone have nothing to
        // send as.
        const sendable = ({ mailbox, addresses, identity }: Row) => {
            if (!mailbox) return []
            return addresses.flatMap(({ domainRecord, ...address }) =>
                domainRecord ? [{ mailbox, address, domain: domainRecord, identity }] : []
            )
        }
        return groupSendableIdentities((rows ?? []).flatMap(sendable))
    }, [rows])
}
