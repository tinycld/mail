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
        // An identity with no inbox (its inbox was deleted outside
        // offboarding) is not live: the server gives it no address.
        const adds = people
            .filter(p => p.inbox !== '' && !existing.has(p.emailName))
            .map(p => `${p.emailName}@${domainName}`)
        return { adds: adds.sort(), removes: [] as string[] }
    }
    const removes = addressesOnDomain
        .filter(a => a.source === 'rule')
        .map(a => `${a.localPart}@${domainName}`)
    return { adds: [] as string[], removes: removes.sort() }
}

const LISTED = 5

function shortList(addresses: string[]) {
    const listed = addresses.slice(0, LISTED).join(', ')
    const rest = addresses.length - LISTED
    if (rest <= 0) return listed
    return `${listed} and ${rest} more`
}

/** Pure: the preview sentence. A big org names only the first few addresses. */
export function coveragePreviewText(preview: { adds: string[]; removes: string[] }) {
    if (preview.adds.length > 0) return `Adds ${preview.adds.length}: ${shortList(preview.adds)}`
    const n = preview.removes.length
    if (n === 1) return `1 address stops receiving mail: ${shortList(preview.removes)}`
    if (n > 1) return `${n} addresses stop receiving mail: ${shortList(preview.removes)}`
    return 'No addresses change.'
}
