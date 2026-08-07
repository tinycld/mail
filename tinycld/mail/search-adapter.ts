import { useOrgHref } from '@tinycld/core/lib/org-routes'
import type { SearchRow } from '@tinycld/core/lib/search/types'
import { useRouter } from 'expo-router'

interface MailSearchHit {
    thread_id: string
    subject: string
    participants: string
    latest_date: string
    mailbox_id: string
}

export function toRow(hit: unknown): Omit<SearchRow, 'slug'> | null {
    const thread = hit as MailSearchHit
    return {
        // Mail returns thread_id rather than id — the palette never sees the
        // difference because this is the only place that shape is read.
        id: thread.thread_id,
        title: thread.subject || '(no subject)',
        subtitle: thread.participants || undefined,
        meta: undefined,
    }
}

export function useSearchActions() {
    const router = useRouter()
    const orgHref = useOrgHref()
    return {
        onSelect: (row: SearchRow) => {
            router.push(orgHref('mail/[id]', { id: row.id }))
        },
    }
}
