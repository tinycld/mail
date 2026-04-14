import { eq } from '@tanstack/db'
import { useMemo } from 'react'
import { useOrgLiveQuery, useStore } from '~/lib/pocketbase'

interface Recipient {
    name: string
    email: string
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
        .map(s => s.trim())
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
    const [contactsCollection] = useStore('contacts')

    const { data: contacts } = useOrgLiveQuery((query, { userOrgId }) =>
        query
            .from({ contacts: contactsCollection })
            .where(({ contacts }) => eq(contacts.owner, userOrgId))
            .orderBy(({ contacts }) => contacts.first_name, 'asc')
    )

    const { committed, committedEmails, activeQuery, committedRaw } = useMemo(
        () => parseCommittedRecipients(formValue),
        [formValue]
    )

    const suggestions = useMemo(() => {
        if (!activeQuery || activeQuery.length < 1 || !contacts) return []
        const q = activeQuery.toLowerCase()
        return contacts.filter(c => {
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
