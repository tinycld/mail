import { and, eq } from '@tanstack/db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useMyLiveQuery } from '@tinycld/core/lib/use-my-live-query'

export function useThreadClassification(threadId: string) {
    const [classificationsCollection, messagesCollection] = useStore(
        'classifications',
        'mail_messages'
    )
    const { data } = useMyLiveQuery(
        (query, { userId }) =>
            query
                .from({ c: classificationsCollection })
                .innerJoin({ m: messagesCollection }, ({ c, m }) => eq(m.id, c.record_id))
                .where(({ c, m }) =>
                    and(
                        eq(c.user, userId),
                        eq(c.error, ''),
                        eq(c.preview, false),
                        eq(m.thread, threadId)
                    )
                )
                .orderBy(({ c }) => c.created, 'desc')
                .select(({ c }) => ({
                    label: c.label_name,
                    confidence: c.confidence,
                    stage: c.stage,
                }))
                .findOne(),
        [threadId]
    )
    if (!data) return undefined
    return {
        label: data.label,
        confidencePct: Math.round(data.confidence * 100),
        stage: data.stage,
    }
}
