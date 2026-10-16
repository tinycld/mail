import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { log } from '@tinycld/core/lib/logger'
import { performMutations, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import { FormErrorSummary, TextInput, Toggle, useForm, zodResolver } from '@tinycld/core/ui/form'
import { View } from 'react-native'
import type { AddressRowData } from './address-items'
import {
    defaultFromAfterRename,
    type EmailNameValues,
    emailNameSchema,
    keptAddresses,
    renameIncompleteMessage,
} from './email-name'

interface RenameIdentity {
    id: string
    emailName: string
    inbox: string
    defaultFrom: string
}

type KeptAddress = ReturnType<typeof keptAddresses>[number]
type RenameForm = Parameters<typeof handleMutationErrorsWithForm<EmailNameValues>>[0]

function useRename(
    identity: RenameIdentity,
    addresses: AddressRowData[],
    form: RenameForm,
    onDone: () => void
) {
    const [identitiesCollection, addressesCollection] = useStore(
        'mail_identities',
        'mail_addresses'
    )
    const insertKept = (k: KeptAddress) =>
        performMutations(function* () {
            yield addressesCollection.insert({
                id: k.newId,
                local_part: k.localPart,
                domain: k.domainId,
                mailbox: identity.inbox,
                source: 'manual',
                is_primary: false,
                via_group: '',
                created: '',
                updated: '',
            })
        })
    const updateDefaultFrom = (defaultFrom: string) =>
        performMutations(function* () {
            yield identitiesCollection.update(identity.id, d => {
                d.default_from = defaultFrom
            })
        })

    return useMutation({
        mutationFn: async (values: EmailNameValues) => {
            const kept = values.keep_old
                ? keptAddresses(addresses, identity.emailName, values.email_name)
                : []
            // The rename must come first: the server frees the old address
            // only when the rename persists, and a manual copy of it is
            // refused while the old name still holds it.
            await performMutations(function* () {
                yield identitiesCollection.update(identity.id, d => {
                    d.email_name = values.email_name
                })
            })
            // The name is saved now. A later failure must say what is
            // missing, not report the whole rename as failed.
            const results = await Promise.allSettled(kept.map(insertKept))
            const notKept = kept.filter((_, i) => results[i].status === 'rejected')
            const newDefault = defaultFromAfterRename(
                identity.defaultFrom,
                kept.filter(k => !notKept.includes(k))
            )
            const defaultFromFailed =
                newDefault !== null &&
                (await updateDefaultFrom(newDefault).then(
                    () => false,
                    () => true
                ))
            if (notKept.length > 0 || defaultFromFailed) {
                log.warn('mail.rename', 'the rename saved, a later step did not', {
                    identity: identity.id,
                    notKept: notKept.map(k => k.email),
                    defaultFromFailed,
                })
            }
            return renameIncompleteMessage(
                notKept.map(k => k.email),
                defaultFromFailed
            )
        },
        onSuccess: incomplete => {
            if (incomplete) {
                form.setError('root', { type: 'manual', message: incomplete })
                return
            }
            onDone()
        },
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
    const onSubmit = handleSubmit(values => {
        // The schema has trimmed and lowercased the name.
        if (values.email_name === identity.emailName) {
            onDone()
            return
        }
        rename.mutate(values)
    })

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
