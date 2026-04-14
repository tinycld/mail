import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { CheckCircle, Circle, Globe, Plus, Trash2 } from 'lucide-react-native'
import { newRecordId } from 'pbtsdb'
import { useState } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'
import { handleMutationErrorsWithForm } from '~/lib/errors'
import { mutation, useMutation } from '~/lib/mutations'
import { useStore } from '~/lib/pocketbase'
import { useThemeColor } from '~/lib/use-app-theme'
import { useOrgInfo } from '~/lib/use-org-info'
import { useSettings } from '~/lib/use-settings'
import { Divider } from '~/ui/divider'
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
    const foregroundColor = useThemeColor('foreground')
    const mutedColor = useThemeColor('muted-foreground')
    const primaryColor = useThemeColor('primary')
    const primaryFgColor = useThemeColor('primary-foreground')
    const backgroundColor = useThemeColor('background')
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
        mutationFn: mutation(function* (data: z.infer<typeof mailSettingsSchema>) {
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
        }),
        onError: handleMutationErrorsWithForm({ setError, getValues }),
    })

    const onSubmit = handleSubmit(data => saveMutation.mutate(data))
    const canSubmit = !saveMutation.isPending && isDirty

    return (
        <ScrollView contentContainerStyle={{ flexGrow: 1 }} style={{ backgroundColor }}>
            <View style={{ flex: 1, padding: 20, gap: 20, maxWidth: 600 }}>
                <View style={{ gap: 8 }}>
                    <Globe size={32} color={primaryColor} />
                    <Text style={{ fontSize: 20, fontWeight: 'bold', color: foregroundColor }}>
                        Mail Provider
                    </Text>
                    <Text style={{ fontSize: 13, color: mutedColor }}>
                        Configure the email provider and domains for your organization.
                    </Text>
                </View>

                <FormErrorSummary errors={errors} isEnabled={isSubmitted} />

                <View style={{ gap: 16 }}>
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
                </View>

                <Pressable
                    onPress={onSubmit}
                    disabled={!canSubmit}
                    style={{
                        backgroundColor: primaryColor,
                        height: 44,
                        borderRadius: 8,
                        alignItems: 'center',
                        justifyContent: 'center',
                        opacity: canSubmit ? 1 : 0.5,
                    }}
                >
                    <Text style={{ fontWeight: '600', color: primaryFgColor }}>
                        {saveMutation.isPending ? 'Saving...' : 'Save'}
                    </Text>
                </Pressable>

                <Divider />

                <DomainsSection orgId={orgId} />
            </View>
        </ScrollView>
    )
}

interface DomainRow {
    id: string
    domain: string
    verified: boolean
}

function DomainsSection({ orgId }: { orgId: string }) {
    const foregroundColor = useThemeColor('foreground')
    const mutedColor = useThemeColor('muted-foreground')
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
        <View style={{ gap: 12 }}>
            <Text style={{ fontSize: 18, fontWeight: 'bold', color: foregroundColor }}>
                Domains
            </Text>
            <Text style={{ fontSize: 13, color: mutedColor }}>
                Add domains your organization can send and receive email on. Mark domains as
                verified once DNS records are configured.
            </Text>

            <NoDomainsBanner isVisible={domainRows.length === 0} />

            {domainRows.map(d => (
                <DomainRowItem key={d.id} domain={d} />
            ))}

            <AddDomainForm orgId={orgId} />
        </View>
    )
}

function NoDomainsBanner({ isVisible }: { isVisible: boolean }) {
    const mutedColor = useThemeColor('muted-foreground')
    if (!isVisible) return null
    return (
        <Text style={{ fontSize: 13, color: mutedColor, fontStyle: 'italic' }}>
            No domains added yet.
        </Text>
    )
}

function DomainRowItem({ domain }: { domain: DomainRow }) {
    const foregroundColor = useThemeColor('foreground')
    const mutedColor = useThemeColor('muted-foreground')
    const borderColor = useThemeColor('border')
    const [domainsCollection] = useStore('mail_domains')
    const [confirming, setConfirming] = useState(false)

    const deleteMutation = useMutation({
        mutationFn: mutation(function* () {
            yield domainsCollection.delete(domain.id)
        }),
        onSuccess: () => setConfirming(false),
    })

    const toggleVerifiedMutation = useMutation({
        mutationFn: mutation(function* () {
            yield domainsCollection.update(domain.id, draft => {
                draft.verified = !domain.verified
            })
        }),
    })

    const VerifiedIcon = domain.verified ? CheckCircle : Circle
    const verifiedColor = domain.verified ? '#16a34a' : mutedColor

    return (
        <View
            style={{
                flexDirection: 'row',
                justifyContent: 'space-between',
                alignItems: 'center',
                borderWidth: 1,
                borderColor,
                borderRadius: 12,
                padding: 12,
            }}
        >
            <View style={{ flexDirection: 'row', gap: 8, alignItems: 'center', flex: 1 }}>
                <Pressable style={{ padding: 4 }} onPress={() => toggleVerifiedMutation.mutate()}>
                    <VerifiedIcon size={18} color={verifiedColor} />
                </Pressable>
                <View>
                    <Text style={{ fontWeight: '600', color: foregroundColor }}>
                        {domain.domain}
                    </Text>
                    <Text style={{ fontSize: 11, color: verifiedColor }}>
                        {domain.verified ? 'Verified' : 'Unverified'}
                    </Text>
                </View>
            </View>

            <DeleteDomainButton
                confirming={confirming}
                onConfirm={() => deleteMutation.mutate()}
                onStartConfirm={() => setConfirming(true)}
                onCancel={() => setConfirming(false)}
            />
        </View>
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
    const _primaryColor = useThemeColor('primary')
    const dangerColor = useThemeColor('danger')

    if (confirming) {
        return (
            <View style={{ flexDirection: 'row', gap: 8 }}>
                <Pressable
                    onPress={onConfirm}
                    style={{
                        paddingHorizontal: 12,
                        paddingVertical: 6,
                        borderRadius: 6,
                        backgroundColor: dangerColor,
                    }}
                >
                    <Text style={{ fontSize: 13, color: '#fff' }}>Remove</Text>
                </Pressable>
                <Pressable
                    onPress={onCancel}
                    style={{
                        paddingHorizontal: 12,
                        paddingVertical: 6,
                        borderRadius: 6,
                    }}
                >
                    <Text style={{ fontSize: 13 }}>Cancel</Text>
                </Pressable>
            </View>
        )
    }

    return (
        <Pressable style={{ padding: 4 }} onPress={onStartConfirm}>
            <Trash2 size={16} color={dangerColor} />
        </Pressable>
    )
}

function AddDomainForm({ orgId }: { orgId: string }) {
    const primaryColor = useThemeColor('primary')
    const primaryFgColor = useThemeColor('primary-foreground')
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
        mutationFn: mutation(function* (data: z.infer<typeof addDomainSchema>) {
            yield domainsCollection.insert({
                id: newRecordId(),
                domain: data.domain,
                org: orgId,
                verified: false,
            })
        }),
        onSuccess: () => reset(),
        onError: handleMutationErrorsWithForm({ setError, getValues }),
    })

    const onSubmit = handleSubmit(data => addMutation.mutate(data))
    const canSubmit = !addMutation.isPending && isDirty

    const addButton = (
        <Pressable
            onPress={onSubmit}
            disabled={!canSubmit}
            style={{
                flexDirection: 'row',
                alignItems: 'center',
                gap: 4,
                backgroundColor: primaryColor,
                paddingHorizontal: 16,
                paddingVertical: 10,
                borderRadius: 8,
                opacity: canSubmit ? 1 : 0.5,
            }}
        >
            <Plus size={16} color={primaryFgColor} />
            <Text style={{ fontWeight: '600', color: primaryFgColor }}>
                {addMutation.isPending ? 'Adding...' : 'Add'}
            </Text>
        </Pressable>
    )

    return (
        <View style={{ gap: 12 }}>
            <FormErrorSummary errors={errors} isEnabled={isSubmitted} />

            <TextInput
                control={control}
                name="domain"
                label="Add Domain"
                placeholder="example.com"
                wrapperProps={{ style: { marginBottom: 0 } }}
                addon={addButton}
            />
        </View>
    )
}
