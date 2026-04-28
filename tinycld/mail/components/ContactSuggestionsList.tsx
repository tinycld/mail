import { eq } from '@tanstack/db'
import { usePackages } from '@tinycld/core/lib/packages/use-packages'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import { type ContactSuggestion, filterContactSuggestions } from '../hooks/useRecipientSuggestions'
import { RecipientSuggestionList } from './RecipientSuggestionList'

interface ContactSuggestionsListProps {
    activeQuery: string
    committedEmails: Set<string>
    onSelect: (contact: ContactSuggestion) => void
}

// Fetches suggestions from the contacts collection. This component is
// only mounted when @tinycld/contacts is linked (see the parent guard),
// so `useStore('contacts')` is safe here — it would throw synchronously
// if the collection were missing.
function ContactSuggestionsFromCollection({
    activeQuery,
    committedEmails,
    onSelect,
}: ContactSuggestionsListProps) {
    // biome-ignore lint/suspicious/noExplicitAny: cross-package soft dependency — see mail/README
    const [contactsCollection] = useStore('contacts' as any) as [any]

    const { data } = useOrgLiveQuery(
        (query, { userOrgId }) =>
            query
                .from({ contacts: contactsCollection })
                // biome-ignore lint/suspicious/noExplicitAny: collection is dynamic
                .where(({ contacts }: any) => eq(contacts.owner, userOrgId))
                // biome-ignore lint/suspicious/noExplicitAny: collection is dynamic
                .orderBy(({ contacts }: any) => contacts.first_name, 'asc'),
        []
    )

    const suggestions = filterContactSuggestions(
        data as ContactSuggestion[] | undefined,
        activeQuery,
        committedEmails
    )
    if (suggestions.length === 0) return null
    return (
        <RecipientSuggestionList
            suggestions={suggestions}
            query={activeQuery}
            onSelect={onSelect}
        />
    )
}

// Runtime gate: only subscribe to the contacts collection when the
// package is installed. usePackages() reads the runtime registry;
// if contacts is absent we render nothing and never call useStore.
export function ContactSuggestionsList(props: ContactSuggestionsListProps) {
    const packages = usePackages()
    const contactsInstalled = packages.some(p => p.slug === 'contacts')
    if (!contactsInstalled) return null
    return <ContactSuggestionsFromCollection {...props} />
}
