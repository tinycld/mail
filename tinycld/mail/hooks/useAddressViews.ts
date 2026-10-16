import { and, eq, not } from '@tanstack/db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useMyLiveQuery } from '@tinycld/core/lib/use-my-live-query'
import { materialize } from 'pbtsdb'
import { useMemo } from 'react'
import { useMailboxFolderCounts } from './useMailboxFolderCounts'

interface RankedAddress {
    id: string
    email: string
    rank: number
}

function localPart(email: string) {
    return email.slice(0, email.lastIndexOf('@'))
}

// Code-unit order, as the server's string compare, so the sidebar and the
// IMAP folder list agree.
function compareText(a: string, b: string) {
    if (a < b) return -1
    return a > b ? 1 : 0
}

/**
 * The addresses that get their own folder under the personal Inbox, in domain
 * rank order, then by local part. The same rule as the IMAP folders
 * (personalAddressViews in server/imap_folders.go): a view of the only
 * visible address is the whole inbox, so with the switch off or fewer than
 * two visible addresses there are no views.
 */
export function visibleAddressViews(input: {
    enabled: boolean
    hidden: string[]
    addresses: RankedAddress[]
}): { id: string; email: string }[] {
    if (!input.enabled) return []
    const visible = input.addresses.filter(a => !input.hidden.includes(a.id))
    if (visible.length < 2) return []
    const ordered = [...visible].sort(
        (a, b) =>
            a.rank - b.rank ||
            compareText(localPart(a.email), localPart(b.email)) ||
            compareText(a.email, b.email)
    )
    return ordered.map(({ id, email }) => ({ id, email }))
}

/** The signed-in person's address views, each with its unread inbox count. */
export function useAddressViews() {
    const [identitiesCollection, addressesCollection, domainsCollection] = useStore(
        'mail_identities',
        'mail_addresses',
        'mail_domains'
    )
    const counts = useMailboxFolderCounts()

    // The identity drives the query; its inbox's addresses and each address's
    // domain are read from their own collections. A retired identity has no
    // inbox, and an address with no mailbox delivers nowhere.
    const { data: rows } = useMyLiveQuery(
        (query, { userId }) =>
            query
                .from({ identity: identitiesCollection })
                .where(({ identity }) =>
                    and(eq(identity.user, userId), not(eq(identity.retired, true)))
                )
                .select(({ identity }) => ({
                    inbox: identity.inbox,
                    enabled: identity.address_views,
                    hidden: identity.hidden_address_views,
                    addresses: materialize(
                        query
                            .from({ address: addressesCollection })
                            .where(({ address }) =>
                                and(
                                    eq(address.mailbox, identity.inbox),
                                    not(eq(address.mailbox, ''))
                                )
                            )
                            .select(({ address }) => ({
                                id: address.id,
                                localPart: address.local_part,
                                domain: materialize(
                                    query
                                        .from({ domain: domainsCollection })
                                        .where(({ domain }) => eq(domain.id, address.domain))
                                        .findOne()
                                ),
                            }))
                    ),
                })),
        { queryKey: ['mail_address_views'] }
    )

    return useMemo(() => {
        const identity = rows?.[0]
        if (!identity) return []
        // An address whose domain is gone, or not loaded yet, has no email.
        const ranked = identity.addresses.flatMap(({ id, localPart, domain }) =>
            domain ? [{ id, email: `${localPart}@${domain.domain}`, rank: domain.rank }] : []
        )
        const views = visibleAddressViews({
            enabled: identity.enabled,
            hidden: identity.hidden,
            addresses: ranked,
        })
        const byAddress = counts.get(identity.inbox)?.inboxByAddress ?? {}
        return views.map(view => ({ ...view, unread: byAddress[view.id]?.unread ?? 0 }))
    }, [rows, counts])
}

export type AddressView = ReturnType<typeof useAddressViews>[number]
