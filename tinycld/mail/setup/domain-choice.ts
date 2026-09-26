export type DomainChoice = 'none' | 'own'

// With no contributed domain options there is nothing to choose between, so
// the own-domain form opens straight away.
export function initialChoice(hasContributedOptions: boolean): DomainChoice {
    return hasContributedOptions ? 'none' : 'own'
}

export function showOwnDomainForm(choice: DomainChoice) {
    return choice === 'own'
}
