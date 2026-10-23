import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { ButtonGroup, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { useEffect } from 'react'
import { toAddressItems } from './address-items'
import type { PersonAddress } from './usePersonEmail'

const pickerSchema = z.object({ default_from: z.string() })

type PickerValues = z.infer<typeof pickerSchema>

const MAIN_DOMAIN_OPTION = { label: "The main domain's address", value: '' }

/** Only an address on a verified domain can send. */
function pickerOptions(addresses: PersonAddress[]) {
    const sendable = toAddressItems(addresses.filter(a => a.verified))
    return [MAIN_DOMAIN_OPTION, ...sendable.map(item => ({ label: item.email, value: item.id }))]
}

export function DefaultFromPicker({
    identityId,
    addresses,
    value,
}: {
    identityId: string
    addresses: PersonAddress[]
    value: string
}) {
    const [identitiesCollection] = useStore('mail_identities')
    const { control, watch, setError, setValue, getValues } = useForm<PickerValues>({
        resolver: zodResolver(pickerSchema),
        // `values`: the picker follows the saved row.
        values: { default_from: value },
    })
    const showFieldError = handleMutationErrorsWithForm({ setError, getValues })
    const save = useMutation({
        mutationFn: mutation(function* (defaultFrom: string) {
            yield identitiesCollection.update(identityId, d => {
                d.default_from = defaultFrom
            })
        }),
        onError: error => {
            // The saved row did not change, so `values` does not re-sync it.
            setValue('default_from', value)
            showFieldError(error)
        },
    })
    const { mutate } = save

    // Saves on change: this is a subscription to the field, not state sync.
    useEffect(() => {
        const subscription = watch((values, { type }) => {
            if (type === 'change') mutate(values.default_from ?? '')
        })
        return () => subscription.unsubscribe()
    }, [watch, mutate])

    return (
        <ButtonGroup
            control={control}
            name="default_from"
            label="Send new mail from"
            options={pickerOptions(addresses)}
        />
    )
}
