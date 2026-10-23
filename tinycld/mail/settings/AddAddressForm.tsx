import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { ButtonIcon, ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import {
    FormErrorSummary,
    RadioInput,
    TextInput,
    useForm,
    zodResolver,
} from '@tinycld/core/ui/form'
import { Plus } from 'lucide-react-native'
import { newRecordId } from 'pbtsdb/core'
import { useMemo } from 'react'
import { View } from 'react-native'
import { type AddressFormValues, addressSchemaFor } from './addressSchema'
import { useAddressRules } from './useAddressRules'

export type AddressTarget =
    | { kind: 'inbox'; mailboxId: string }
    | { kind: 'group'; groupId: string }

// An inbox's own owner may use their email name; no inbox owns a group address.
function ownInboxOf(target: AddressTarget) {
    return target.kind === 'inbox' ? target.mailboxId : null
}

export function AddAddressForm({ target, onDone }: { target: AddressTarget; onDone: () => void }) {
    const [addressesCollection] = useStore('mail_addresses')
    const { rules, domainOptions } = useAddressRules(ownInboxOf(target))
    const schema = useMemo(() => addressSchemaFor(rules), [rules])
    const {
        control,
        handleSubmit,
        setError,
        getValues,
        formState: { errors, isSubmitted, isValid },
    } = useForm<AddressFormValues>({
        mode: 'onChange',
        resolver: zodResolver(schema),
        defaultValues: { local_part: '', domain: '' },
    })

    const create = useMutation({
        mutationFn: mutation(function* (data: AddressFormValues) {
            yield addressesCollection.insert({
                id: newRecordId(),
                local_part: data.local_part,
                domain: data.domain,
                mailbox: target.kind === 'inbox' ? target.mailboxId : '',
                source: 'manual',
                is_primary: false,
                via_group: '',
                group: target.kind === 'group' ? target.groupId : '',
                created: '',
                updated: '',
            })
        }),
        onSuccess: onDone,
        onError: handleMutationErrorsWithForm({ setError, getValues }),
    })
    const onSubmit = handleSubmit(d => create.mutate(d))

    return (
        <View className="gap-2">
            <FormErrorSummary errors={errors} isEnabled={isSubmitted} />
            <TextInput control={control} name="local_part" label="Address" placeholder="sales" />
            <RadioInput control={control} name="domain" label="Domain" options={domainOptions} />
            <ServerActionButton
                size="sm"
                onPress={onSubmit}
                isDisabled={!isValid || create.isPending}
            >
                <ButtonIcon as={Plus} />
                <ButtonText>Add address</ButtonText>
            </ServerActionButton>
        </View>
    )
}
