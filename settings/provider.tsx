import { eq } from '@tanstack/db'
import { useMutation as useReactQueryMutation } from '@tanstack/react-query'
import { CheckCircle, Globe, Loader2, Plus, RefreshCw, Trash2, XCircle } from 'lucide-react-native'
import { newRecordId } from 'pbtsdb/core'
import { useState } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'
import { errorToString, handleMutationErrorsWithForm } from '~/lib/errors'
import { mutation, useMutation } from '~/lib/mutations'
import { pb, useStore } from '~/lib/pocketbase'
import { useThemeColor } from '~/lib/use-app-theme'
import { useOrgInfo } from '~/lib/use-org-info'
import { useOrgLiveQuery } from '~/lib/use-org-live-query'
import { useSettings } from '~/lib/use-settings'
import { Divider } from '~/ui/divider'
import { FormErrorSummary, SelectInput, TextInput, useForm, z, zodResolver } from '~/ui/form'
import type { MailDomainVerificationDetails } from '../types'

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
            <View className="flex-1 gap-5 p-5" style={{ maxWidth: 600 }}>
                <View className="gap-2">
                    <Globe size={32} color={primaryColor} />
                    <Text style={{ fontSize: 20, fontWeight: 'bold', color: foregroundColor }}>
                        Mail Provider
                    </Text>
                    <Text style={{ fontSize: 13, color: mutedColor }}>
                        Configure the email provider and domains for your organization.
                    </Text>
                </View>

                <FormErrorSummary errors={errors} isEnabled={isSubmitted} />

                <View className="gap-4">
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
                    className={`items-center justify-center rounded-lg h-11 ${canSubmit ? 'opacity-100' : 'opacity-50'}`}
                    style={{ backgroundColor: primaryColor }}
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
    mx_verified: boolean
    inbound_domain_verified: boolean
    spf_verified: boolean
    dkim_verified: boolean
    return_path_verified: boolean
    last_checked_at: string
    verification_details: MailDomainVerificationDetails | null
}

function DomainsSection({ orgId }: { orgId: string }) {
    const foregroundColor = useThemeColor('foreground')
    const mutedColor = useThemeColor('muted-foreground')
    const [domainsCollection] = useStore('mail_domains')

    const { data: domains } = useOrgLiveQuery((query, { orgId }) =>
        query
            .from({ mail_domains: domainsCollection })
            .where(({ mail_domains }) => eq(mail_domains.org, orgId))
            .orderBy(({ mail_domains }) => mail_domains.created, 'asc')
    )

    const domainRows: DomainRow[] = (domains ?? []).map(d => ({
        id: d.id,
        domain: d.domain,
        verified: d.verified,
        mx_verified: d.mx_verified,
        inbound_domain_verified: d.inbound_domain_verified,
        spf_verified: d.spf_verified,
        dkim_verified: d.dkim_verified,
        return_path_verified: d.return_path_verified,
        last_checked_at: d.last_checked_at,
        verification_details: d.verification_details ?? null,
    }))

    return (
        <View className="gap-3">
            <Text style={{ fontSize: 18, fontWeight: 'bold', color: foregroundColor }}>
                Domains
            </Text>
            <Text style={{ fontSize: 13, color: mutedColor }}>
                Add domains your organization can send and receive email on. Click Verify to
                re-check DNS and provider status.
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

    const verifyMutation = useReactQueryMutation({
        mutationFn: async () => {
            await pb.send(`/api/mail/domains/${domain.id}/verify`, { method: 'POST' })
        },
    })

    const verifyErrorMessage = verifyMutation.error ? errorToString(verifyMutation.error) : null

    const VerifiedIcon = domain.verified ? CheckCircle : XCircle
    const verifiedColor = domain.verified ? '#16a34a' : '#dc2626'

    return (
        <View
            className="gap-3 border rounded-xl p-3"
            style={{
                borderColor,
            }}
        >
            <View className="flex-row justify-between items-center">
                <View className="flex-row gap-2 items-center flex-1">
                    <VerifiedIcon size={18} color={verifiedColor} />
                    <View>
                        <Text style={{ fontWeight: '600', color: foregroundColor }}>
                            {domain.domain}
                        </Text>
                        <Text style={{ fontSize: 11, color: verifiedColor }}>
                            {domain.verified ? 'Verified' : 'Unverified'}
                        </Text>
                    </View>
                </View>

                <View className="flex-row gap-2 items-center">
                    <VerifyButton
                        isPending={verifyMutation.isPending}
                        onPress={() => verifyMutation.mutate()}
                    />
                    <DeleteDomainButton
                        confirming={confirming}
                        onConfirm={() => deleteMutation.mutate()}
                        onStartConfirm={() => setConfirming(true)}
                        onCancel={() => setConfirming(false)}
                    />
                </View>
            </View>

            <DomainVerificationPanel domain={domain} />

            <VerifyErrorBanner message={verifyErrorMessage} />

            <LastCheckedLabel lastCheckedAt={domain.last_checked_at} mutedColor={mutedColor} />
        </View>
    )
}

function VerifyButton({ isPending, onPress }: { isPending: boolean; onPress: () => void }) {
    const primaryColor = useThemeColor('primary')
    const primaryFgColor = useThemeColor('primary-foreground')
    const Icon = isPending ? Loader2 : RefreshCw
    return (
        <Pressable
            onPress={isPending ? undefined : onPress}
            disabled={isPending}
            className={`flex-row items-center gap-1 px-3 rounded-md py-1.5 ${isPending ? 'opacity-60' : 'opacity-100'}`}
            style={{ backgroundColor: primaryColor }}
        >
            <Icon size={14} color={primaryFgColor} />
            <Text style={{ fontSize: 12, fontWeight: '600', color: primaryFgColor }}>
                {isPending ? 'Verifying…' : 'Verify'}
            </Text>
        </Pressable>
    )
}

function VerifyErrorBanner({ message }: { message: string | null }) {
    const dangerColor = useThemeColor('danger')
    if (!message) return null
    return (
        <Text style={{ fontSize: 11, color: dangerColor }}>
            Verification request failed: {message}
        </Text>
    )
}

function LastCheckedLabel({
    lastCheckedAt,
    mutedColor,
}: {
    lastCheckedAt: string
    mutedColor: string
}) {
    if (!lastCheckedAt) return null
    return (
        <Text style={{ fontSize: 10, color: mutedColor, fontStyle: 'italic' }}>
            Last checked {new Date(lastCheckedAt).toLocaleString()}
        </Text>
    )
}

function DomainVerificationPanel({ domain }: { domain: DomainRow }) {
    const details = domain.verification_details
    const mxActual = details?.mx?.actual?.join(', ') || details?.mx?.error || 'no MX records'
    const postmarkDetail = details?.postmark?.error
        ? details.postmark.error
        : details?.postmark?.server_domain
          ? `Postmark server InboundDomain: ${details.postmark.server_domain || '(empty)'}`
          : null
    const outboundError = details?.outbound?.error

    return (
        <View className="gap-1">
            <CheckRow
                label="Inbound MX"
                ok={domain.mx_verified}
                hint={
                    domain.mx_verified
                        ? 'MX points to inbound.postmarkapp.com'
                        : `expected inbound.postmarkapp.com — found: ${mxActual}`
                }
            />
            <CheckRow
                label="Postmark Inbound Domain"
                ok={domain.inbound_domain_verified}
                hint={
                    domain.inbound_domain_verified
                        ? postmarkDetail || 'Postmark server InboundDomain matches this domain'
                        : postmarkDetail ||
                          'Set this domain as the server InboundDomain in Postmark'
                }
            />
            <CheckRow
                label="SPF"
                ok={domain.spf_verified}
                hint={outboundError || 'outbound sending'}
                advisory
            />
            <CheckRow
                label="DKIM"
                ok={domain.dkim_verified}
                hint={outboundError || 'outbound sending'}
                advisory
            />
            <CheckRow
                label="Return-Path"
                ok={domain.return_path_verified}
                hint={outboundError || 'outbound sending'}
                advisory
            />
        </View>
    )
}

function CheckRow({
    label,
    ok,
    hint,
    advisory,
}: {
    label: string
    ok: boolean
    hint: string
    advisory?: boolean
}) {
    const mutedColor = useThemeColor('muted-foreground')
    const foregroundColor = useThemeColor('foreground')
    const Icon = ok ? CheckCircle : XCircle
    const iconColor = ok ? '#16a34a' : advisory ? mutedColor : '#dc2626'
    return (
        <View className="flex-row gap-2 items-start">
            <Icon size={14} color={iconColor} style={{ marginTop: 2 }} />
            <View className="flex-1">
                <Text style={{ fontSize: 12, color: foregroundColor }}>
                    {label}
                    {advisory ? (
                        <Text style={{ fontSize: 10, color: mutedColor }}> (optional)</Text>
                    ) : null}
                </Text>
                <Text style={{ fontSize: 11, color: mutedColor }}>{hint}</Text>
            </View>
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
            <View className="flex-row gap-2">
                <Pressable
                    onPress={onConfirm}
                    className="px-3 rounded-md"
                    style={{
                        paddingVertical: 6,
                        backgroundColor: dangerColor,
                    }}
                >
                    <Text style={{ fontSize: 13, color: '#fff' }}>Remove</Text>
                </Pressable>
                <Pressable
                    onPress={onCancel}
                    className="px-3 rounded-md"
                    style={{
                        paddingVertical: 6,
                    }}
                >
                    <Text style={{ fontSize: 13 }}>Cancel</Text>
                </Pressable>
            </View>
        )
    }

    return (
        <Pressable className="p-1" onPress={onStartConfirm}>
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
                mx_verified: false,
                inbound_domain_verified: false,
                spf_verified: false,
                dkim_verified: false,
                return_path_verified: false,
                last_checked_at: '',
                verification_details: null,
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
            className={`flex-row items-center gap-1 px-4 rounded-lg py-2.5 ${canSubmit ? 'opacity-100' : 'opacity-50'}`}
            style={{ backgroundColor: primaryColor }}
        >
            <Plus size={16} color={primaryFgColor} />
            <Text style={{ fontWeight: '600', color: primaryFgColor }}>
                {addMutation.isPending ? 'Adding...' : 'Add'}
            </Text>
        </Pressable>
    )

    return (
        <View className="gap-3">
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
