import { useLiveQuery } from '@tanstack/react-db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useMemo } from 'react'
import type { AddressRules } from './addressSchema'

// The rules come from every address, domain and identity in the org. This is
// an admin screen and the sets are small (people × domains), and loading them
// up front keeps the schema independent of what is typed, so the form has one
// resolver that does not change while the user types.
//
// ownInbox is the inbox the new address goes to: its owner may use their own
// email name. A new inbox, or a group address, has no such owner.
export function useAddressRules(ownInbox: string | null) {
    const [addressesCollection, domainsCollection, identitiesCollection] = useStore(
        'mail_addresses',
        'mail_domains',
        'mail_identities'
    )
    const { data: domains, isReady } = useLiveQuery({
        queryKey: ['mail_address_rule_domains'],
        query: query =>
            query.from({ domain: domainsCollection }).orderBy(({ domain }) => domain.rank, 'asc'),
    })
    const { data: addresses } = useLiveQuery({
        queryKey: ['mail_address_rule_addresses'],
        query: query =>
            query.from({ address: addressesCollection }).select(({ address }) => ({
                localPart: address.local_part,
                domain: address.domain,
            })),
    })
    const { data: identities } = useLiveQuery({
        queryKey: ['mail_address_rule_identities'],
        query: query =>
            query.from({ identity: identitiesCollection }).select(({ identity }) => ({
                emailName: identity.email_name,
                inbox: identity.inbox,
                retired: identity.retired,
            })),
    })

    const rules: AddressRules = useMemo(
        () => ({
            taken: new Set((addresses ?? []).map(a => `${a.localPart}@${a.domain}`)),
            reservedNames: new Set(
                (identities ?? [])
                    .filter(i => !i.retired && i.inbox !== ownInbox)
                    .map(i => i.emailName)
            ),
            // Same rule as server/addresses.go domainHasRule.
            ruleDomainIds: new Set(
                (domains ?? [])
                    .filter(d => d.coverage === 'everyone' || d.coverage === 'groups')
                    .map(d => d.id)
            ),
        }),
        [addresses, identities, domains, ownInbox]
    )
    const domainOptions = useMemo(
        () => (domains ?? []).map(d => ({ label: d.domain, value: d.id })),
        [domains]
    )
    return { rules, domainOptions, isReady }
}
