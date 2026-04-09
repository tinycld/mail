import { useLiveQuery } from '@tanstack/react-db'
import { useCallback, useMemo } from 'react'
import { useStore } from '~/lib/pocketbase'

type LabelInfo = { id: string; name: string; color: string }

export function useLabels() {
    const [labelsCollection] = useStore('mail_labels')

    const { data: allLabels } = useLiveQuery(
        query => query.from({ mail_labels: labelsCollection }),
        []
    )

    const labels = allLabels ?? []

    const labelMap = useMemo(() => {
        const map = new Map<string, LabelInfo>()
        for (const l of labels) map.set(l.id, l)
        return map
    }, [labels])

    const labelsForIds = useCallback(
        (ids: string[]) => ids.map(id => labelMap.get(id)).filter((l): l is LabelInfo => l != null),
        [labelMap]
    )

    return { labels, labelMap, labelsForIds }
}
