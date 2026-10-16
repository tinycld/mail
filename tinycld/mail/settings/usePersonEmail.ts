import { coalesce, eq, type InitialQueryBuilder } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import type { AddressRowData } from './address-items'

type Stores = ReturnType<
    typeof useStore<['mail_identities', 'mail_addresses', 'mail_domains', 'groups']>
>

export interface PersonEmailCollections {
    identities: Stores[0]
    addresses: Stores[1]
    domains: Stores[2]
    groups: Stores[3]
}

/**
 * The one joined query of a person's email: their identity, the addresses of
 * the identity's inbox, each address's domain, and the group that gave a rule
 * address. Shared by the settings screens, which add their own key and filter
 * source (useLiveQuery here, useMyLiveQuery for "my own" screens).
 */
export function personEmailQuery(
    query: InitialQueryBuilder,
    collections: PersonEmailCollections,
    userId: string
) {
    return query
        .from({ identity: collections.identities })
        .join({ address: collections.addresses }, ({ identity, address }) =>
            eq(address.mailbox, identity.inbox)
        )
        .join({ domain: collections.domains }, ({ address, domain }) =>
            eq(address.domain, domain.id)
        )
        .join({ group: collections.groups }, ({ address, group }) =>
            eq(address.via_group, group.id)
        )
        .where(({ identity }) => eq(identity.user, userId))
        .select(({ identity, address, domain, group }) => ({
            identityId: identity.id,
            emailName: identity.email_name,
            inbox: identity.inbox,
            defaultFrom: identity.default_from,
            retired: identity.retired,
            // Left joins: a person with no covered domain has an inbox and no address.
            addressId: address?.id,
            localPart: address?.local_part,
            domainId: domain?.id,
            domainName: domain?.domain,
            source: address?.source,
            isPrimary: address?.is_primary,
            rank: domain?.rank,
            groupName: coalesce(group?.name, null),
        }))
}

export type PersonEmailRow = NonNullable<ReturnType<typeof usePersonEmailRows>['data']>[number]

function usePersonEmailRows(userId: string) {
    const [identities, addresses, domains, groups] = useStore(
        'mail_identities',
        'mail_addresses',
        'mail_domains',
        'groups'
    )
    return useLiveQuery({
        queryKey: ['mail_person_email', userId],
        query: query => personEmailQuery(query, { identities, addresses, domains, groups }, userId),
    })
}

function toAddressRow(row: PersonEmailRow): AddressRowData | null {
    const { addressId, localPart, domainId, domainName, source, isPrimary, rank } = row
    if (!addressId || !localPart || !domainId || !domainName) return null
    return {
        id: addressId,
        localPart,
        domainId,
        domainName,
        source: source ?? 'manual',
        isPrimary: isPrimary ?? false,
        rank: rank ?? 0,
        groupName: row.groupName,
    }
}

/** Folds the joined rows into the identity and its addresses (the identity survives zero addresses). */
export function toPersonEmail(rows: PersonEmailRow[]) {
    const first = rows[0]
    const identity = first
        ? {
              id: first.identityId,
              emailName: first.emailName,
              inbox: first.inbox,
              defaultFrom: first.defaultFrom,
              retired: first.retired,
          }
        : null
    return { identity, addresses: rows.map(toAddressRow).filter(a => a !== null) }
}

/** A person's identity and every address of its inbox, from one live query. */
export function usePersonEmail(userId: string) {
    const { data, isReady } = usePersonEmailRows(userId)
    return { ...toPersonEmail(data ?? []), isReady }
}
