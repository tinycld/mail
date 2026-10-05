import { and, eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useQuery } from '@tanstack/react-query'
import { pb, useStore } from '@tinycld/core/lib/pocketbase'
import { serverFetch } from '@tinycld/core/lib/server-fetch'
import { useMemo } from 'react'
import type { MailMessages } from '../types'
import type { DraftContext } from './useComposeState'

/**
 * The draft message behind a thread the user opened from the list. The list
 * carries only ids, so the draft row loads here (one on-demand request for
 * `thread = id && delivery_status = 'draft'`), and the body file follows.
 *
 * mail_messages' viewRule is member-scoped, so the body needs the short-lived
 * token from pb.files.getToken() (mirrors EmailBody.tsx). The body is a file,
 * not a collection row, so React Query fetches it directly.
 */
export function useDraftMessage(
    context: DraftContext | null
): { message: MailMessages; htmlBody: string } | null {
    const [messagesCollection] = useStore('mail_messages')
    const threadId = context?.threadId ?? ''

    const { data: drafts } = useLiveQuery({
        query: query =>
            threadId
                ? query
                      .from({ m: messagesCollection })
                      .where(({ m }) => and(eq(m.thread, threadId), eq(m.delivery_status, 'draft')))
                : undefined,
    })
    const message = drafts?.[0] as MailMessages | undefined

    const { data: htmlBody, isLoading: bodyLoading } = useQuery({
        queryKey: ['mail_draft_body', message?.id, message?.body_html],
        enabled: !!message?.body_html,
        queryFn: async () => {
            if (!message?.body_html) return ''
            const token = await pb.files.getToken()
            const url = pb.files.getURL(
                { collectionId: 'mail_messages', id: message.id },
                message.body_html,
                { token }
            )
            return (await serverFetch(url)).text()
        },
    })

    return useMemo(() => {
        if (!message) return null
        if (message.body_html && bodyLoading) return null
        return { message, htmlBody: htmlBody ?? '' }
    }, [message, htmlBody, bodyLoading])
}
