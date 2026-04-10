export { useLabels } from '~/hooks/useLabels'

import { useLabelsForRecord } from '~/hooks/useLabels'

export function useThreadLabels(threadStateId: string) {
    return useLabelsForRecord(threadStateId, 'mail_thread_state')
}
