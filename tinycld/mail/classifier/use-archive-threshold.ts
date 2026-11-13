import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useSettings } from '@tinycld/core/lib/use-settings'
import { useForm, zodResolver } from '@tinycld/core/ui/form'
import { newRecordId } from 'pbtsdb/core'
import {
    type ArchiveThresholdValues,
    archiveThresholdSchema,
    DEFAULT_ARCHIVE_THRESHOLD,
} from '../settings/archive-threshold-schema'

const KEY = 'classifier.archive_threshold'

export function useArchiveThreshold() {
    const [settingsCollection] = useStore('settings')
    const rows = useSettings('mail')
    const existing = rows.find(r => r.key === KEY)
    const stored = existing ? String(existing.value) : String(DEFAULT_ARCHIVE_THRESHOLD)
    const form = useForm<ArchiveThresholdValues>({
        resolver: zodResolver(archiveThresholdSchema),
        values: { value: stored },
        mode: 'onChange',
    })
    const { setError, getValues, reset } = form

    const save = useMutation({
        mutationFn: function* (values: ArchiveThresholdValues) {
            // The server reads a JSON string or number; a string keeps it exact.
            const value = String(Number(values.value))
            if (existing) {
                yield settingsCollection.update(existing.id, draft => {
                    draft.value = value
                })
            } else {
                yield settingsCollection.insert({ id: newRecordId(), app: 'mail', key: KEY, value })
            }
        },
        onSuccess: (_result, values) => reset(values),
        onError: handleMutationErrorsWithForm({ setError, getValues }),
    })

    const submit = form.handleSubmit(values => save.mutate(values))
    return { form, submit: () => void submit(), isPending: save.isPending }
}
