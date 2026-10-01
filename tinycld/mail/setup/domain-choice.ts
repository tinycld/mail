// The Email domain step lays its choices out as tabs: one per domain option
// another package contributes, then "Your own domain". A tab's value is the
// contributor's slug, or OWN_DOMAIN for the own-domain form.
export const OWN_DOMAIN = 'own'

export type DomainChoice = string

export interface DomainTab {
    value: DomainChoice
    label: string
}

export interface ContributedOption {
    contributorSlug: string
    label?: string
}

const OWN_TAB: DomainTab = { value: OWN_DOMAIN, label: 'Your own domain' }

// A contribution without a label is named after its package, so a missing
// manifest field shows as an odd tab rather than an empty one.
export function domainTabs(contributed: readonly ContributedOption[]): DomainTab[] {
    const tabs = contributed.map(c => ({
        value: c.contributorSlug,
        label: c.label ?? c.contributorSlug,
    }))
    return [...tabs, OWN_TAB]
}

// The first contributed option is the recommended one; with none, the
// own-domain form is the whole step.
export function initialChoice(tabs: readonly DomainTab[]): DomainChoice {
    return tabs[0]?.value ?? OWN_DOMAIN
}

// The tab picked on an earlier visit wins while it still exists; a package
// removed since then falls back to the default.
export function currentChoice(
    tabs: readonly DomainTab[],
    picked: DomainChoice | null
): DomainChoice {
    if (picked && tabs.some(tab => tab.value === picked)) return picked
    return initialChoice(tabs)
}

// One tab is no choice: the bar is shown only when there is something to pick.
export function showDomainTabs(tabs: readonly DomainTab[]): boolean {
    return tabs.length > 1
}

export function showOwnDomainForm(choice: DomainChoice): boolean {
    return choice === OWN_DOMAIN
}
