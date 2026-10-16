import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import { FormErrorSummary, TextInput, Toggle, useForm, zodResolver } from '@tinycld/core/ui/form'
import { newRecordId } from 'pbtsdb/core'
import { View } from 'react-native'
import type { AddressRowData } from './address-items'
import {
    defaultFromAfterRename,
    type EmailNameValues,
    emailNameSchema,
    keptAddresses,
} from './email-name'

interface RenameIdentity {
    id: string
    emailName: string
    inbox: string
    defaultFrom: string
}

function useRename(
    identity: RenameIdentity,
    addresses: AddressRowData[],
    form: {
        setError: Parameters<typeof handleMutationErrorsWithForm<EmailNameValues>>[0]['setError']
        getValues: () => EmailNameValues
    },
    onDone: () => void
) {
    const [identitiesCollection, addressesCollection] = useStore(
        'mail_identities',
        'mail_addresses'
    )
    return useMutation({
        mutationFn: mutation(function* (values: EmailNameValues) {
            const kept = values.keep_old ? keptAddresses(addresses, identity.emailName) : []
            // The rename must come first: the server frees the old address
            // only when the rename persists, and a manual copy of it is
            // refused while the old name still holds it.
            yield identitiesCollection.update(identity.id, d => {
                d.email_name = values.email_name
            })
            const newIds = new Map(kept.map(k => [k.oldId, newRecordId()]))
            yield kept.map(k =>
                addressesCollection.insert({
                    id: newIds.get(k.oldId) ?? newRecordId(),
                    local_part: k.localPart,
                    domain: k.domainId,
                    mailbox: identity.inbox,
                    source: 'manual',
                    is_primary: false,
                    via_group: '',
                    created: '',
                    updated: '',
                })
            )
            const newDefault = defaultFromAfterRename(identity.defaultFrom, kept, newIds)
            if (newDefault === null) return
            yield identitiesCollection.update(identity.id, d => {
                d.default_from = newDefault
            })
        }),
        onSuccess: onDone,
        onError: handleMutationErrorsWithForm(form),
    })
}

export function EmailNameForm({
    identity,
    addresses,
    onDone,
}: {
    identity: RenameIdentity
    addresses: AddressRowData[]
    onDone: () => void
}) {
    const {
        control,
        handleSubmit,
        setError,
        getValues,
        formState: { errors, isSubmitted },
    } = useForm<EmailNameValues>({
        mode: 'onChange',
        resolver: zodResolver(emailNameSchema),
        defaultValues: { email_name: identity.emailName, keep_old: true },
    })
    const rename = useRename(identity, addresses, { setError, getValues }, onDone)
    const onSubmit = handleSubmit(values => rename.mutate(values))

    return (
        <View className="gap-2">
            <FormErrorSummary errors={errors} isEnabled={isSubmitted} />
            <TextInput control={control} name="email_name" label="Email name" />
            <Toggle
                control={control}
                name="keep_old"
                label="Keep the old addresses as extra addresses"
            />
            <ServerActionButton size="sm" onPress={onSubmit} isDisabled={rename.isPending}>
                <ButtonText>Rename</ButtonText>
            </ServerActionButton>
        </View>
    )
}
