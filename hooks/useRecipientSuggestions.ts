import { eq } from '@tanstack/db'
import { useMemo } from 'react'
import { usePackages } from '~/lib/packages/use-packages'
import { useStore } from '~/lib/pocketbase'
import { useOrgLiveQuery } from '~/lib/use-org-live-query'

interface Recipient {
    name: string
    email: string
}

// Local shape for the contacts package's records — declared here so this
// hook compiles even when @tinycld/contacts isn't linked into the
// MergedSchema. Fields are guarded behind usePackages() at runtime so we
// never query a non-existent collection.
interface ContactSuggestion {
    id: string
    first_name: string
    last_name: string
    email: string
    owner: string
}

const namedEmailPattern = /^(.+)\s*<([^\s@]+@[^\s@]+\.[^\s@]+)>$/

function parseCommittedRecipients(formValue: string): {
    committed: Recipient[]
    committedEmails: Set<string>
    activeQuery: string
    committedRaw: string
} {
    const lastCommaIndex = formValue.lastIndexOf(',')
    if (lastCommaIndex === -1) {
        return {
            committed: [],
            committedEmails: new Set(),
            activeQuery: formValue.trim(),
            committedRaw: '',
        }
    }

    const committedRaw = formValue.slice(0, lastCommaIndex + 1)
    const activeQuery = formValue.slice(lastCommaIndex + 1).trim()
    const segments = committedRaw
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean)

    const committed: Recipient[] = []
    const committedEmails = new Set<string>()
    for (const seg of segments) {
        const match = seg.match(namedEmailPattern)
        if (match) {
            committed.push({ name: match[1].trim(), email: match[2] })
            committedEmails.add(match[2].toLowerCase())
        } else {
            committed.push({ name: '', email: seg })
            committedEmails.add(seg.toLowerCase())
        }
    }

    return { committed, committedEmails, activeQuery, committedRaw }
}

export function useRecipientSuggestions(formValue: string) {
    // Contacts is an optional sibling package. usePackages() reflects the
    // runtime registry; bail with empty suggestions if it's not installed
    // (the useStore call below would target a missing collection).
    const packages = usePackages()
    const contactsInstalled = packages.some((p) => p.slug === 'contacts')

    // useStore is typed against MergedSchema, which only contains 'contacts'
    // when @tinycld/contacts is linked into core. Cast through `any` so this
    // hook compiles in mail's standalone CI; the runtime contactsInstalled
    // guard prevents the query from running when the collection is absent.
    // biome-ignore lint/suspicious/noExplicitAny: cross-package soft dependency
    const [contactsCollection] = useStore('contacts' as any) as [any]

    const { data: contactsData } = useOrgLiveQuery(
        (query, { userOrgId }) => {
            if (!contactsInstalled) return null
            return (
                query
                    .from({ contacts: contactsCollection })
                    // biome-ignore lint/suspicious/noExplicitAny: collection is dynamic
                    .where(({ contacts }: any) => eq(contacts.owner, userOrgId))
                    // biome-ignore lint/suspicious/noExplicitAny: collection is dynamic
                    .orderBy(({ contacts }: any) => contacts.first_name, 'asc')
            )
        },
        [contactsInstalled]
    )
    const contacts = contactsData as ContactSuggestion[] | undefined

    const { committed, committedEmails, activeQuery, committedRaw } = useMemo(
        () => parseCommittedRecipients(formValue),
        [formValue]
    )

    const suggestions = useMemo(() => {
        if (!activeQuery || activeQuery.length < 1 || !contacts) return []
        const q = activeQuery.toLowerCase()
        return contacts.filter((c) => {
            if (committedEmails.has(c.email?.toLowerCase())) return false
            const fullName = `${c.first_name} ${c.last_name}`.toLowerCase()
            return fullName.includes(q) || c.email?.toLowerCase().includes(q)
        })
    }, [activeQuery, contacts, committedEmails])

    const showSuggestions = suggestions.length > 0

    return {
        committedRecipients: committed,
        activeQuery,
        committedRaw,
        suggestions,
        showSuggestions,
    }
}
