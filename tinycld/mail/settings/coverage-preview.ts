export type Coverage = 'everyone' | 'manual'

interface PreviewParams {
    target: Coverage
    domainName: string
    people: { emailName: string; inbox: string }[]
    addressesOnDomain: { localPart: string; mailbox: string; source: string }[]
}

/**
 * Pure: what changing a domain's rule does, in the same terms the server's
 * reconciler uses (server/rule_addresses.go). Switching to everyone adds each
 * person's email name unless that address already exists; switching to manual
 * removes every rule-made address. Addresses added by hand are never removed.
 */
export function coverageChangePreview({
    target,
    domainName,
    people,
    addressesOnDomain,
}: PreviewParams) {
    const existing = new Set(addressesOnDomain.map(a => a.localPart))
    if (target === 'everyone') {
        const adds = people
            .filter(p => !existing.has(p.emailName))
            .map(p => `${p.emailName}@${domainName}`)
        return { adds: adds.sort(), removes: [] as string[] }
    }
    const removes = addressesOnDomain
        .filter(a => a.source === 'rule')
        .map(a => `${a.localPart}@${domainName}`)
    return { adds: [] as string[], removes: removes.sort() }
}
