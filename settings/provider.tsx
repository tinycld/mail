import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { CheckCircle, Circle, Globe, Plus, Trash2 } from 'lucide-react-native'
import { newRecordId } from 'pbtsdb'
import { useState } from 'react'
import { Button, H4, ScrollView, Separator, SizableText, useTheme, XStack, YStack } from 'tamagui'
import { handleMutationErrorsWithForm } from '~/lib/errors'
import { useMutation } from '~/lib/mutations'
import { useStore } from '~/lib/pocketbase'
import { useOrgInfo } from '~/lib/use-org-info'
import { useSettings } from '~/lib/use-settings'
import { FormErrorSummary, SelectInput, TextInput, useForm, z, zodResolver } from '~/ui/form'

const PROVIDER_OPTIONS = [{ label: 'Postmark', value: 'postmark' }]

const mailSettingsSchema = z.object({
    provider: z.string().min(1, 'Provider is required'),
    postmark_server_token: z.string(),
    postmark_account_token: z.string(),
})

const addDomainSchema = z.object({
    domain: z
        .string()
        .min(1, 'Domain is required')
        .regex(
            /^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$/,
            'Enter a valid domain'
        ),
})

export default function ProviderSettings() {
    const theme = useTheme()
    const { orgId } = useOrgInfo()
    const settings = useSettings('mail', orgId)
    const [settingsCollection] = useStore('settings')

    const settingsMap = new Map(settings.map(s => [s.key, s]))

    const {
        control,
        handleSubmit,
        setError,
        getValues,
        formState: { errors, isSubmitted, isDirty },
    } = useForm({
        mode: 'onChange',
        resolver: zodResolver(mailSettingsSchema),
        values: {
            provider: (settingsMap.get('provider')?.value as string) ?? 'postmark',
            postmark_server_token:
                (settingsMap.get('postmark_server_token')?.value as string) ?? '',
            postmark_account_token:
                (settingsMap.get('postmark_account_token')?.value as string) ?? '',
        },
    })

    const saveMutation = useMutation({
        mutationFn: function* (data: z.infer<typeof mailSettingsSchema>) {
            const entries = [
                { key: 'provider', value: data.provider },
                { key: 'postmark_server_token', value: data.postmark_server_token },
                { key: 'postmark_account_token', value: data.postmark_account_token },
            ]
            for (const entry of entries) {
                const existing = settingsMap.get(entry.key)
                if (existing) {
                    yield settingsCollection.update(existing.id, draft => {
                        draft.value = entry.value
                    })
                } else {
                    yield settingsCollection.insert({
                        id: newRecordId(),
                        app: 'mail',
                        key: entry.key,
                        value: entry.value,
                        org: orgId,
                    })
                }
            }
        },
        onError: handleMutationErrorsWithForm({ setError, getValues }),
    })

    const onSubmit = handleSubmit(data => saveMutation.mutate(data))
    const canSubmit = !saveMutation.isPending && isDirty

    return (
        <ScrollView contentContainerStyle={{ flexGrow: 1 }} backgroundColor="$background">
            <YStack flex={1} padding="$5" gap="$5" maxWidth={600}>
                <YStack gap="$2">
                    <Globe size={32} color={theme.colorFocus.val} />
                    <SizableText size="$6" fontWeight="bold" color="$color">
                        Mail Provider
                    </SizableText>
                    <SizableText size="$3" color="$color8">
                        Configure the email provider and domains for your organization.
                    </SizableText>
                </YStack>

                <FormErrorSummary errors={errors} isEnabled={isSubmitted} />

                <YStack gap="$4">
                    <SelectInput
                        control={control}
                        name="provider"
                        label="Provider"
                        options={PROVIDER_OPTIONS}
                    />
                    <TextInput
                        control={control}
                        name="postmark_server_token"
                        label="Postmark Server Token"
                        secureTextEntry
                    />
                    <TextInput
                        control={control}
                        name="postmark_account_token"
                        label="Postmark Account Token"
                        secureTextEntry
                    />
                </YStack>

                <Button
                    theme="accent"
                    size="$4"
                    onPress={onSubmit}
                    disabled={!canSubmit}
                    opacity={canSubmit ? 1 : 0.5}
                >
                    <Button.Text fontWeight="600">
                        {saveMutation.isPending ? 'Saving...' : 'Save'}
                    </Button.Text>
                </Button>

                <Separator />

                <DomainsSection orgId={orgId} />
            </YStack>
        </ScrollView>
    )
}

interface DomainRow {
    id: string
    domain: string
    verified: boolean
}

function DomainsSection({ orgId }: { orgId: string }) {
    const [domainsCollection] = useStore('mail_domains')

    const { data: domains } = useLiveQuery(
        query =>
            query
                .from({ mail_domains: domainsCollection })
                .where(({ mail_domains }) => eq(mail_domains.org, orgId))
                .orderBy(({ mail_domains }) => mail_domains.created, 'asc'),
        [orgId]
    )

    const domainRows: DomainRow[] = (domains ?? []).map(d => ({
        id: d.id,
        domain: d.domain,
        verified: d.verified,
    }))

    return (
        <YStack gap="$3">
            <H4 color="$color">Domains</H4>
            <SizableText size="$3" color="$color8">
                Add domains your organization can send and receive email on. Mark domains as
                verified once DNS records are configured.
            </SizableText>

            <NoDomainsBanner isVisible={domainRows.length === 0} />

            {domainRows.map(d => (
                <DomainRowItem key={d.id} domain={d} />
            ))}

            <AddDomainForm orgId={orgId} />
        </YStack>
    )
}

function NoDomainsBanner({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <SizableText size="$3" color="$color8" fontStyle="italic">
            No domains added yet.
        </SizableText>
    )
}

function DomainRowItem({ domain }: { domain: DomainRow }) {
    const [domainsCollection] = useStore('mail_domains')
    const [confirming, setConfirming] = useState(false)

    const deleteMutation = useMutation({
        mutationFn: function* () {
            yield domainsCollection.delete(domain.id)
        },
        onSuccess: () => setConfirming(false),
    })

    const toggleVerifiedMutation = useMutation({
        mutationFn: function* () {
            yield domainsCollection.update(domain.id, draft => {
                draft.verified = !domain.verified
            })
        },
    })

    const VerifiedIcon = domain.verified ? CheckCircle : Circle
    const verifiedColor = domain.verified ? '$green10' : '$color8'

    return (
        <XStack
            justifyContent="space-between"
            alignItems="center"
            borderWidth={1}
            borderColor="$borderColor"
            borderRadius="$3"
            padding="$3"
        >
            <XStack gap="$2" alignItems="center" flex={1}>
                <Button size="$2" chromeless onPress={() => toggleVerifiedMutation.mutate()}>
                    <VerifiedIcon size={18} color={verifiedColor} />
                </Button>
                <YStack>
                    <SizableText fontWeight="600" color="$color">
                        {domain.domain}
                    </SizableText>
                    <SizableText size="$1" color={verifiedColor}>
                        {domain.verified ? 'Verified' : 'Unverified'}
                    </SizableText>
                </YStack>
            </XStack>

            <DeleteDomainButton
                confirming={confirming}
                onConfirm={() => deleteMutation.mutate()}
                onStartConfirm={() => setConfirming(true)}
                onCancel={() => setConfirming(false)}
            />
        </XStack>
    )
}

function DeleteDomainButton({
    confirming,
    onConfirm,
    onStartConfirm,
    onCancel,
}: {
    confirming: boolean
    onConfirm: () => void
    onStartConfirm: () => void
    onCancel: () => void
}) {
    if (confirming) {
        return (
            <XStack gap="$2">
                <Button size="$2" theme="red" onPress={onConfirm}>
                    <Button.Text>Remove</Button.Text>
                </Button>
                <Button size="$2" onPress={onCancel}>
                    <Button.Text>Cancel</Button.Text>
                </Button>
            </XStack>
        )
    }

    return (
        <Button size="$2" chromeless onPress={onStartConfirm}>
            <Trash2 size={16} color="$red10" />
        </Button>
    )
}

function AddDomainForm({ orgId }: { orgId: string }) {
    const [domainsCollection] = useStore('mail_domains')

    const {
        control,
        handleSubmit,
        setError,
        getValues,
        reset,
        formState: { errors, isSubmitted, isDirty },
    } = useForm({
        mode: 'onChange',
        resolver: zodResolver(addDomainSchema),
        defaultValues: { domain: '' },
    })

    const addMutation = useMutation({
        mutationFn: function* (data: z.infer<typeof addDomainSchema>) {
            yield domainsCollection.insert({
                id: newRecordId(),
                domain: data.domain,
                org: orgId,
                verified: false,
            })
        },
        onSuccess: () => reset(),
        onError: handleMutationErrorsWithForm({ setError, getValues }),
    })

    const onSubmit = handleSubmit(data => addMutation.mutate(data))
    const canSubmit = !addMutation.isPending && isDirty

    const addButton = (
        <Button
            theme="accent"
            size="$4"
            onPress={onSubmit}
            disabled={!canSubmit}
            opacity={canSubmit ? 1 : 0.5}
        >
            <XStack gap="$1" alignItems="center">
                <Plus size={16} />
                <Button.Text fontWeight="600">
                    {addMutation.isPending ? 'Adding...' : 'Add'}
                </Button.Text>
            </XStack>
        </Button>
    )

    return (
        <YStack gap="$3">
            <FormErrorSummary errors={errors} isEnabled={isSubmitted} />

            <TextInput
                control={control}
                name="domain"
                label="Add Domain"
                placeholder="example.com"
                wrapperProps={{ marginBottom: 0 }}
                addon={addButton}
            />
        </YStack>
    )
}
