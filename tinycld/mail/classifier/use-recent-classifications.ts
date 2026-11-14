import { and, eq } from '@tanstack/db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useMyLiveQuery } from '@tinycld/core/lib/use-my-live-query'
import { materialize } from 'pbtsdb'
import { actionText, confidenceText, stageText } from './classification-display'

export function useRecentClassifications() {
    const [classifications, messages] = useStore('classifications', 'mail_messages')
    const { data = [] } = useMyLiveQuery(
        (query, { userId }) =>
            query
                .from({ c: classifications })
                .where(({ c }) => and(eq(c.user, userId), eq(c.app, 'mail')))
                .orderBy(({ c }) => c.created, 'desc')
                .limit(25)
                .select(({ c }) => ({
                    id: c.id,
                    label: c.label_name,
                    confidence: c.confidence,
                    stage: c.stage,
                    action: c.action,
                    preview: c.preview,
                    error: c.error,
                    message: materialize(
                        query
                            .from({ m: messages })
                            .where(({ m }) => eq(m.id, c.record_id))
                            .findOne()
                    ),
                })),
        []
    )
    return data.map(row => ({
        id: row.id,
        subject: row.message?.subject || '(message removed)',
        // label_name is the snapshot, so the row still reads right after the
        // taxonomy label it came from was deleted.
        label: row.label || 'No label',
        confidence: confidenceText(row.confidence, row.error),
        stage: stageText(row.stage),
        action: actionText(row),
    }))
}
