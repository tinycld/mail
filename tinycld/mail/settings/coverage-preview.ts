export type Coverage = 'everyone' | 'groups' | 'manual'

interface PreviewParams {
    target: Coverage
    // The groups a groups rule names; ignored for the other rules.
    targetGroups: string[]
    domainName: string
    people: { userId: string; emailName: string; inbox: string }[]
    // userId → the ids of the groups that person is a member of.
    memberships: Map<string, string[]>
    addressesOnDomain: { localPart: string; mailbox: string; source: string }[]
}

/**
 * Pure: what changing a domain's rule does, in the same terms the server's
 * reconciler uses (server/rule_addresses.go). Switching to everyone adds each
 * person's email name unless that address already exists; switching to groups
 * does the same for members of the chosen groups and removes the rule-made
 * addresses of everyone else; switching to manual removes every rule-made
 * address. Addresses added by hand are never removed.
 */
export function coverageChangePreview({
    target,
    targetGroups,
    domainName,
    people,
    memberships,
    addressesOnDomain,
}: PreviewParams) {
    const existing = new Set(addressesOnDomain.map(a => a.localPart))
    if (target === 'groups') {
        const members = people.filter(
            p => p.inbox !== '' && targetGroups.some(g => memberships.get(p.userId)?.includes(g))
        )
        const memberInboxes = new Set(members.map(p => p.inbox))
        const adds = members
            .filter(p => !existing.has(p.emailName))
            .map(p => `${p.emailName}@${domainName}`)
        const removes = addressesOnDomain
            .filter(a => a.source === 'rule' && !memberInboxes.has(a.mailbox))
            .map(a => `${a.localPart}@${domainName}`)
        return { adds: adds.sort(), removes: removes.sort() }
    }
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

function addsText(adds: string[]) {
    if (adds.length === 0) return ''
    return `Adds ${adds.length}: ${shortList(adds)}`
}

function removesText(removes: string[]) {
    const n = removes.length
    if (n === 0) return ''
    if (n === 1) return `1 address stops receiving mail: ${shortList(removes)}`
    return `${n} addresses stop receiving mail: ${shortList(removes)}`
}

/** Pure: the preview sentences. A big org names only the first few addresses. */
export function coveragePreviewText(preview: { adds: string[]; removes: string[] }) {
    const sentences = [addsText(preview.adds), removesText(preview.removes)].filter(Boolean)
    if (sentences.length === 0) return 'No addresses change.'
    // A sentence ends in an address, so a period marks where the next starts.
    return sentences.join('. ')
}
