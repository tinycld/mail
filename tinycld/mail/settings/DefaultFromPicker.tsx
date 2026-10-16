import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { SelectInput, useForm } from '@tinycld/core/ui/form'
import { useEffect } from 'react'
import { type AddressRowData, toAddressItems } from './address-items'

interface PickerValues {
    default_from: string
}

const MAIN_DOMAIN_OPTION = { label: "The main domain's address", value: '' }

export function DefaultFromPicker({
    identityId,
    addresses,
    value,
}: {
    identityId: string
    addresses: AddressRowData[]
    value: string
}) {
    const [identitiesCollection] = useStore('mail_identities')
    const { control, watch, setError, getValues } = useForm<PickerValues>({
        // `values`: the picker follows the saved row.
        values: { default_from: value },
    })
    const save = useMutation({
        mutationFn: mutation(function* (defaultFrom: string) {
            yield identitiesCollection.update(identityId, d => {
                d.default_from = defaultFrom
            })
        }),
        onError: handleMutationErrorsWithForm({ setError, getValues }),
    })
    const { mutate } = save

    // Saves on change: this is a subscription to the field, not state sync.
    useEffect(() => {
        const subscription = watch((values, { type }) => {
            if (type === 'change') mutate(values.default_from ?? '')
        })
        return () => subscription.unsubscribe()
    }, [watch, mutate])

    const options = [
        MAIN_DOMAIN_OPTION,
        ...toAddressItems(addresses).map(item => ({ label: item.email, value: item.id })),
    ]
    return (
        <SelectInput
            control={control}
            name="default_from"
            label="Send new mail from"
            options={options}
        />
    )
}
