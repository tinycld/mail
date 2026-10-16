import { and, eq, not } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import {
    FormErrorSummary,
    RadioInput,
    useForm,
    useWatch,
    z,
    zodResolver,
} from '@tinycld/core/ui/form'
import { Text, View } from 'react-native'
import { type Coverage, coverageChangePreview, coveragePreviewText } from './coverage-preview'

const coverageSchema = z.object({ coverage: z.enum(['everyone', 'manual']) })

const OPTIONS = [
    {
        value: 'everyone',
        label: 'Everyone',
        description: 'Each person gets their email name on this domain.',
    },
    { value: 'manual', label: 'Only people I add', description: 'Add each address by hand.' },
]

function useCoveragePreview(domainId: string, domainName: string, target: Coverage) {
    const [identitiesCollection, usersCollection, addressesCollection] = useStore(
        'mail_identities',
        'users',
        'mail_addresses'
    )
    const { data: people } = useLiveQuery(query =>
        query
            .from({ identity: identitiesCollection })
            .innerJoin({ user: usersCollection }, ({ identity, user }) =>
                eq(identity.user, user.id)
            )
            // A retired identity belongs to an offboarded person: it has no
            // inbox and a tombstone email name, so it never gets an address.
            .where(({ user, identity }) =>
                and(not(eq(user.role, 'guest')), eq(identity.retired, false))
            )
            .select(({ identity }) => ({
                emailName: identity.email_name,
                inbox: identity.inbox,
            }))
    )
    const { data: addressesOnDomain } = useLiveQuery(query =>
        query
            .from({ address: addressesCollection })
            .where(({ address }) => eq(address.domain, domainId))
            .select(({ address }) => ({
                localPart: address.local_part,
                mailbox: address.mailbox,
                source: address.source,
            }))
    )
    return coverageChangePreview({
        target,
        domainName,
        people: people ?? [],
        addressesOnDomain: addressesOnDomain ?? [],
    })
}

export function DomainCoverageControl({
    domainId,
    domainName,
    coverage,
}: {
    domainId: string
    domainName: string
    coverage: Coverage
}) {
    const [domainsCollection] = useStore('mail_domains')
    const {
        control,
        handleSubmit,
        reset,
        setError,
        getValues,
        formState: { errors, isSubmitted },
    } = useForm({
        mode: 'onChange',
        resolver: zodResolver(coverageSchema),
        // `values`, not defaultValues: when the domain row changes (this
        // Apply, or another admin), the form follows the saved rule.
        values: { coverage },
    })
    const target = useWatch({ control, name: 'coverage' }) as Coverage
    const preview = useCoveragePreview(domainId, domainName, target)
    const isChanged = target !== coverage

    const save = useMutation({
        mutationFn: mutation(function* (data: { coverage: Coverage }) {
            yield domainsCollection.update(domainId, d => {
                d.coverage = data.coverage
            })
        }),
        onSuccess: () => reset(getValues()),
        onError: handleMutationErrorsWithForm({ setError, getValues }),
    })
    const onConfirm = handleSubmit(data => save.mutate(data))

    return (
        <View className="gap-2">
            <RadioInput
                control={control}
                name="coverage"
                label="Who gets an address here?"
                options={OPTIONS}
            />
            <FormErrorSummary errors={errors} isEnabled={isSubmitted} />
            <ConfirmCoverage
                isVisible={isChanged}
                text={coveragePreviewText(preview)}
                isPending={save.isPending}
                onConfirm={onConfirm}
                onCancel={() => reset({ coverage })}
            />
        </View>
    )
}

function ConfirmCoverage({
    isVisible,
    text,
    isPending,
    onConfirm,
    onCancel,
}: {
    isVisible: boolean
    text: string
    isPending: boolean
    onConfirm: () => void
    onCancel: () => void
}) {
    if (!isVisible) return null
    return (
        <View className="gap-2 rounded-lg border border-border p-2">
            <Text className="text-foreground" style={{ fontSize: 13 }}>
                {text}
            </Text>
            <View className="flex-row gap-2">
                <ServerActionButton size="sm" onPress={onConfirm} isDisabled={isPending}>
                    <ButtonText>Apply</ButtonText>
                </ServerActionButton>
                <ServerActionButton size="sm" variant="outline" onPress={onCancel}>
                    <ButtonText>Cancel</ButtonText>
                </ServerActionButton>
            </View>
        </View>
    )
}
