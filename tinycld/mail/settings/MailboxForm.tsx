import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { ButtonIcon, ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import {
    FormErrorSummary,
    MultiSelectInput,
    RadioInput,
    TextInput,
    useForm,
    zodResolver,
} from '@tinycld/core/ui/form'
import { Plus } from 'lucide-react-native'
import { newRecordId } from 'pbtsdb/core'
import { type ReactNode, useMemo } from 'react'
import { Text, View } from 'react-native'
import type { AddressRules } from './addressSchema'
import { newMailboxRecords } from './mailbox-records'
import {
    type MailboxFormValues,
    mailboxSchema,
    type SharedMailboxFormValues,
    sharedMailboxSchemaFor,
} from './mailbox-schema'
import { useAddressRules } from './useAddressRules'

type DomainOption = { label: string; value: string }

const DISPLAY_NAME_HINT = 'shown in the From: header and in the admin list'

function useCreateMailbox({
    type,
    userId,
    onSuccess,
    onError,
}: {
    type: 'shared' | 'personal'
    userId: string
    onSuccess: () => void
    onError: (error: unknown) => void
}) {
    const [mailboxesCollection, membersCollection, addressesCollection] = useStore(
        'mail_mailboxes',
        'mail_mailbox_members',
        'mail_addresses'
    )
    return useMutation({
        mutationFn: mutation(function* (data: {
            address: string
            display_name: string
            domains: readonly string[]
        }) {
            const { mailbox, member, addresses } = newMailboxRecords({
                address: data.address,
                displayName: data.display_name,
                domains: data.domains,
                type,
                userId,
                newId: newRecordId,
            })
            yield mailboxesCollection.insert(mailbox)
            yield membersCollection.insert(member)
            yield addresses.map(a => addressesCollection.insert(a))
        }),
        onSuccess,
        onError,
    })
}

// Every domain starts checked: a team address such as support@ is usually
// wanted on each of the org's domains, and an admin can uncheck the rest.
export function SharedMailboxCreateForm({
    userId,
    onDone,
}: {
    userId: string
    onDone: () => void
}) {
    const { rules, domainOptions, isReady } = useAddressRules(null)
    // The form reads its defaults once, so it mounts after the domains load.
    if (!isReady) return null
    return (
        <SharedForm rules={rules} domainOptions={domainOptions} userId={userId} onDone={onDone} />
    )
}

function SharedForm({
    rules,
    domainOptions,
    userId,
    onDone,
}: {
    rules: AddressRules
    domainOptions: DomainOption[]
    userId: string
    onDone: () => void
}) {
    const domainNames = useMemo(
        () => new Map(domainOptions.map(d => [d.value, d.label])),
        [domainOptions]
    )
    const schema = useMemo(() => sharedMailboxSchemaFor(rules, domainNames), [rules, domainNames])
    const {
        control,
        handleSubmit,
        setError,
        getValues,
        watch,
        reset,
        formState: { errors, isSubmitted, isValid },
    } = useForm<SharedMailboxFormValues>({
        mode: 'onChange',
        resolver: zodResolver(schema),
        defaultValues: {
            address: '',
            domains: domainOptions.map(d => d.value),
            display_name: '',
        },
    })
    const createMutation = useCreateMailbox({
        type: 'shared',
        userId,
        onSuccess: () => {
            reset()
            onDone()
        },
        onError: handleMutationErrorsWithForm({ setError, getValues }),
    })
    const onSubmit = handleSubmit(d => createMutation.mutate(d))
    const values = watch()
    const chosenNames = (values.domains ?? []).map(id => domainNames.get(id) ?? '')

    return (
        <FormLayout
            errors={errors}
            isSubmitted={isSubmitted}
            preview={{
                address: values.address,
                domainName: chosenNames[0] ?? '',
                displayName: values.display_name,
            }}
            submitLabel={createMutation.isPending ? 'Creating…' : 'Create inbox'}
            canSubmit={!createMutation.isPending && isValid}
            onSubmit={onSubmit}
        >
            <TextInput
                control={control}
                name="address"
                label="Address"
                placeholder="support"
                hint={addressHint(values.address, chosenNames)}
            />
            <DomainChoices isVisible={domainOptions.length > 1}>
                <MultiSelectInput
                    control={control}
                    name="domains"
                    label="Domains"
                    options={domainOptions}
                    hint="The inbox gets this address on each checked domain."
                />
            </DomainChoices>
            <TextInput
                control={control}
                name="display_name"
                label="Display name"
                placeholder="Support Team"
                hint={DISPLAY_NAME_HINT}
            />
        </FormLayout>
    )
}

// A personal inbox has one address. The person's email name on each domain
// with a rule comes from that rule, so a second address here would clash.
export function PersonalMailboxCreateForm({
    domainOptions,
    userId,
    onDone,
    defaults,
}: {
    domainOptions: DomainOption[]
    userId: string
    onDone: () => void
    defaults: Pick<MailboxFormValues, 'address' | 'display_name'>
}) {
    const {
        control,
        handleSubmit,
        setError,
        getValues,
        watch,
        reset,
        formState: { errors, isSubmitted, isValid },
    } = useForm<MailboxFormValues>({
        mode: 'onChange',
        resolver: zodResolver(mailboxSchema),
        defaultValues: { ...defaults, domain: domainOptions[0]?.value ?? '' },
    })
    const createMutation = useCreateMailbox({
        type: 'personal',
        userId,
        onSuccess: () => {
            reset()
            onDone()
        },
        onError: handleMutationErrorsWithForm({ setError, getValues }),
    })
    const onSubmit = handleSubmit(({ domain, ...rest }) =>
        createMutation.mutate({ ...rest, domains: [domain] })
    )
    const values = watch()
    const domainName = domainOptions.find(d => d.value === values.domain)?.label ?? ''

    return (
        <FormLayout
            errors={errors}
            isSubmitted={isSubmitted}
            preview={{
                address: values.address,
                domainName,
                displayName: values.display_name,
            }}
            submitLabel={createMutation.isPending ? 'Creating…' : 'Create inbox'}
            // Not gated on isDirty: prefilled defaults are a valid submission as-is.
            canSubmit={!createMutation.isPending && isValid}
            onSubmit={onSubmit}
        >
            <TextInput
                control={control}
                name="address"
                label="Address"
                placeholder="support"
                hint={addressHint(values.address, [domainName])}
            />
            <DomainChoices isVisible={domainOptions.length > 1}>
                <RadioInput
                    control={control}
                    name="domain"
                    label="Domain"
                    options={domainOptions}
                />
            </DomainChoices>
            <TextInput
                control={control}
                name="display_name"
                label="Display name"
                placeholder="Support Team"
                hint={DISPLAY_NAME_HINT}
            />
        </FormLayout>
    )
}

// The domain is fixed after creation; other domains are added as addresses.
export function MailboxEditForm({
    mailboxId,
    initial,
    domainName,
    onDone,
}: {
    mailboxId: string
    initial: MailboxFormValues
    domainName: string
    onDone: () => void
}) {
    const [mailboxesCollection] = useStore('mail_mailboxes')

    const {
        control,
        handleSubmit,
        setError,
        getValues,
        watch,
        formState: { errors, isSubmitted, isDirty, isValid },
    } = useForm<MailboxFormValues>({
        mode: 'onChange',
        resolver: zodResolver(mailboxSchema),
        defaultValues: initial,
    })

    const editMutation = useMutation({
        mutationFn: mutation(function* (data: MailboxFormValues) {
            yield mailboxesCollection.update(mailboxId, draft => {
                draft.address = data.address
                draft.display_name = data.display_name
                draft.name = data.display_name
            })
        }),
        onSuccess: onDone,
        onError: handleMutationErrorsWithForm({ setError, getValues }),
    })

    const onSubmit = handleSubmit(d => editMutation.mutate(d))
    const values = watch()

    return (
        <FormLayout
            errors={errors}
            isSubmitted={isSubmitted}
            preview={{
                address: values.address,
                domainName,
                displayName: values.display_name,
            }}
            submitLabel={editMutation.isPending ? 'Saving…' : 'Save changes'}
            canSubmit={!editMutation.isPending && isDirty && isValid}
            onSubmit={onSubmit}
        >
            <TextInput
                control={control}
                name="address"
                label="Address"
                placeholder="support"
                hint={addressHint(values.address, [domainName])}
            />
            <TextInput
                control={control}
                name="display_name"
                label="Display name"
                placeholder="Support Team"
                hint={DISPLAY_NAME_HINT}
            />
        </FormLayout>
    )
}

function addressHint(address: string, domainNames: string[]) {
    const local = address || '…'
    const names = domainNames.length > 0 ? domainNames : ['…']
    return `will be: ${names.map(name => `${local}@${name || '…'}`).join(', ')}`
}

function DomainChoices({ isVisible, children }: { isVisible: boolean; children: ReactNode }) {
    if (!isVisible) return null
    return <>{children}</>
}

interface FormLayoutProps {
    errors: Parameters<typeof FormErrorSummary>[0]['errors']
    isSubmitted: boolean
    preview: { address: string; domainName: string; displayName: string }
    submitLabel: string
    canSubmit: boolean
    onSubmit: () => void
    children: ReactNode
}

function FormLayout({
    errors,
    isSubmitted,
    preview,
    submitLabel,
    canSubmit,
    onSubmit,
    children,
}: FormLayoutProps) {
    const previewInitial = (preview.displayName || preview.address || '?')
        .trim()
        .charAt(0)
        .toUpperCase()

    return (
        <View className="gap-4">
            <FormErrorSummary
                errors={errors}
                isEnabled={isSubmitted}
                testID="mailbox-form-errors"
            />
            {children}
            <View className="flex-row gap-3 items-center rounded-lg p-3 border border-border">
                <View
                    className="items-center justify-center rounded-lg bg-primary/10"
                    style={{ width: 30, height: 30 }}
                >
                    <Text className="text-primary" style={{ fontWeight: '700', fontSize: 13 }}>
                        {previewInitial}
                    </Text>
                </View>
                <View className="flex-1" style={{ minWidth: 0 }}>
                    <Text className="text-foreground" style={{ fontSize: 13, fontWeight: '600' }}>
                        {preview.address || '…'}
                        <Text className="text-muted-foreground" style={{ fontWeight: '500' }}>
                            @{preview.domainName || '…'}
                        </Text>
                    </Text>
                    <Text className="text-muted-foreground" style={{ fontSize: 11.5 }}>
                        {preview.displayName || 'Display name'}
                    </Text>
                </View>
                <Text
                    className="text-muted-foreground"
                    style={{
                        fontSize: 10,
                        textTransform: 'uppercase',
                        fontWeight: '600',
                        letterSpacing: 0.6,
                    }}
                >
                    Preview
                </Text>
            </View>
            <ServerActionButton
                testID="mailbox-form-submit"
                className="h-11"
                onPress={onSubmit}
                isDisabled={!canSubmit}
            >
                <ButtonIcon as={Plus} />
                <ButtonText>{submitLabel}</ButtonText>
            </ServerActionButton>
        </View>
    )
}
