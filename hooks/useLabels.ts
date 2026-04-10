export { useLabels } from '~/ui/hooks/useLabels'

import { useLabelsForRecord } from '~/ui/hooks/useLabels'

export function useThreadLabels(threadStateId: string) {
    return useLabelsForRecord(threadStateId, 'mail_thread_state')
}
