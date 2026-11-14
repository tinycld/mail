import { and, eq, inArray } from '@tanstack/db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useMyLiveQuery } from '@tinycld/core/lib/use-my-live-query'

// Bounded by the thread's own message ids, so the query never loads or
// subscribes to the rest of the user's classifications.
export function useThreadClassification(messageIds: string[]) {
    const [classificationsCollection] = useStore('classifications')
    const idsKey = messageIds.join(',')
    const { data } = useMyLiveQuery(
        (query, { userId }) =>
            query
                .from({ c: classificationsCollection })
                .where(({ c }) =>
                    and(
                        inArray(c.record_id, messageIds),
                        eq(c.user, userId),
                        eq(c.error, ''),
                        eq(c.preview, false)
                    )
                )
                .orderBy(({ c }) => c.created, 'desc')
                .select(({ c }) => ({
                    label: c.label_name,
                    confidence: c.confidence,
                    stage: c.stage,
                }))
                .findOne(),
        [idsKey]
    )
    if (!data) return undefined
    return {
        label: data.label,
        confidencePct: Math.round(data.confidence * 100),
        stage: data.stage,
    }
}
