import { useLiveQuery } from '@tanstack/react-db'
import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { ButtonIcon, ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import {
    ButtonGroup,
    FormErrorSummary,
    TextInput,
    useForm,
    zodResolver,
} from '@tinycld/core/ui/form'
import { Plus } from 'lucide-react-native'
import { newRecordId } from 'pbtsdb/core'
import { useMemo } from 'react'
import { View } from 'react-native'
import { type AddressFormValues, addressSchemaFor } from './addressSchema'

// The rules come from every address, domain and identity in the org. This is
// an admin screen and the sets are small (people × domains), and loading them
// up front keeps the schema independent of what is typed, so the form has one
// resolver that does not change while the user types.
export type AddressTarget =
    | { kind: 'inbox'; mailboxId: string }
    | { kind: 'group'; groupId: string }

// An inbox's own owner may use their email name; no inbox owns a group address.
function ownInboxOf(target: AddressTarget) {
    return target.kind === 'inbox' ? target.mailboxId : null
}

function useAddressRules(target: AddressTarget) {
    const ownInbox = ownInboxOf(target)
    const [addressesCollection, domainsCollection, identitiesCollection] = useStore(
        'mail_addresses',
        'mail_domains',
        'mail_identities'
    )
    const { data: domains } = useLiveQuery({
        queryKey: ['mail_address_rule_domains'],
        query: query =>
            query.from({ domain: domainsCollection }).orderBy(({ domain }) => domain.rank, 'asc'),
    })
    const { data: addresses } = useLiveQuery({
        queryKey: ['mail_address_rule_addresses'],
        query: query =>
            query.from({ address: addressesCollection }).select(({ address }) => ({
                localPart: address.local_part,
                domain: address.domain,
            })),
    })
    const { data: identities } = useLiveQuery({
        queryKey: ['mail_address_rule_identities'],
        query: query =>
            query.from({ identity: identitiesCollection }).select(({ identity }) => ({
                emailName: identity.email_name,
                inbox: identity.inbox,
                retired: identity.retired,
            })),
    })

    const schema = useMemo(
        () =>
            addressSchemaFor({
                taken: new Set((addresses ?? []).map(a => `${a.localPart}@${a.domain}`)),
                reservedNames: new Set(
                    (identities ?? [])
                        .filter(i => !i.retired && i.inbox !== ownInbox)
                        .map(i => i.emailName)
                ),
                // Same rule as server/addresses.go domainHasRule.
                ruleDomainIds: new Set(
                    (domains ?? [])
                        .filter(d => d.coverage === 'everyone' || d.coverage === 'groups')
                        .map(d => d.id)
                ),
            }),
        [addresses, identities, domains, ownInbox]
    )
    const domainOptions = (domains ?? []).map(d => ({ label: d.domain, value: d.id }))
    return { schema, domainOptions }
}

export function AddAddressForm({ target, onDone }: { target: AddressTarget; onDone: () => void }) {
    const [addressesCollection] = useStore('mail_addresses')
    const { schema, domainOptions } = useAddressRules(target)
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
            <ButtonGroup control={control} name="domain" label="Domain" options={domainOptions} />
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
