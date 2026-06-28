import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useMutation as useReactQueryMutation } from '@tanstack/react-query'
import { HelpIcon } from '@tinycld/core/components/help/HelpIcon'
import { errorToString, handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { pb, useStore } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useOrgInfo } from '@tinycld/core/lib/use-org-info'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import { FormErrorSummary, TextInput, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import {
    CheckCircle,
    Copy,
    Globe,
    Loader2,
    Plus,
    RefreshCw,
    Trash2,
    XCircle,
} from 'lucide-react-native'
import { newRecordId } from 'pbtsdb/core'
import { useState } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'
import type { MailDomainVerificationDetails } from '../types'

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
    const primaryColor = useThemeColor('primary')
    const { orgId } = useOrgInfo()
    const [systemSettings] = useStore('system_settings')

    const { data: sysRows = [] } = useLiveQuery(query => query.from({ s: systemSettings }))
    const provider =
        (sysRows.find(r => r.key === 'mail.provider')?.value as 'postmark' | 'smtp') || 'postmark'

    return (
        <ScrollView contentContainerStyle={{ flexGrow: 1 }} className="bg-background">
            <View className="flex-1 gap-5 p-5" style={{ maxWidth: 600 }}>
                <ProviderHeader primaryColor={primaryColor} />

                <DomainsSection orgId={orgId} provider={provider} />
            </View>
        </ScrollView>
    )
}

function ProviderHeader({ primaryColor }: { primaryColor: string }) {
    return (
        <View className="gap-2">
            <Globe size={32} color={primaryColor} />
            <View className="flex-row items-center gap-2">
                <Text className="text-foreground" style={{ fontSize: 20, fontWeight: 'bold' }}>
                    Mail Domains
                </Text>
                <HelpIcon topic="mail:provider-setup" size={18} />
            </View>
            <Text className="text-muted-foreground" style={{ fontSize: 13 }}>
                Manage the domains your organization can send and receive email on.
            </Text>
        </View>
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

function DomainsSection({ orgId, provider }: { orgId: string; provider: 'postmark' | 'smtp' }) {
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
            <Text className="text-foreground" style={{ fontSize: 18, fontWeight: 'bold' }}>
                Domains
            </Text>
            <Text className="text-muted-foreground" style={{ fontSize: 13 }}>
                Add domains your organization can send and receive email on. Click Verify to
                re-check DNS and provider status.
            </Text>

            <NoDomainsBanner isVisible={domainRows.length === 0} />

            {domainRows.map(d => (
                <DomainRowItem key={d.id} domain={d} provider={provider} />
            ))}

            <AddDomainForm orgId={orgId} />
        </View>
    )
}

function NoDomainsBanner({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Text className="text-muted-foreground" style={{ fontSize: 13, fontStyle: 'italic' }}>
            No domains added yet.
        </Text>
    )
}

function DomainRowItem({ domain, provider }: { domain: DomainRow; provider: 'postmark' | 'smtp' }) {
    const mutedColor = useThemeColor('muted-foreground')
    const successColor = useThemeColor('success')
    const dangerColor = useThemeColor('danger')
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
    const verifiedColor = domain.verified ? successColor : dangerColor

    return (
        <View className="gap-3 border border-border rounded-xl p-3">
            <View className="flex-row justify-between items-center">
                <View className="flex-row gap-2 items-center flex-1">
                    <VerifiedIcon size={18} color={verifiedColor} />
                    <View>
                        <Text className="text-foreground" style={{ fontWeight: '600' }}>
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

            <DomainVerificationPanel domain={domain} provider={provider} />

            <PostmarkWebhookURLsBlock isVisible={provider === 'postmark'} domainId={domain.id} />

            <VerifyErrorBanner message={verifyErrorMessage} />

            <LastCheckedLabel lastCheckedAt={domain.last_checked_at} mutedColor={mutedColor} />
        </View>
    )
}

function PostmarkWebhookURLsBlock({
    isVisible,
    domainId,
}: {
    isVisible: boolean
    domainId: string
}) {
    if (!isVisible) return null
    return <WebhookURLs domainId={domainId} />
}

function WebhookURLs({ domainId }: { domainId: string }) {
    const mutedColor = useThemeColor('muted-foreground')
    const foregroundColor = useThemeColor('foreground')
    const borderColor = useThemeColor('border')
    const surfaceColor = useThemeColor('surface')
    const [urls, setUrls] = useState<{ inbound: string; bounces: string } | null>(null)
    const [copied, setCopied] = useState<string | null>(null)

    const fetchUrls = useReactQueryMutation({
        mutationFn: async () => {
            const res = await pb.send(`/api/mail/domains/${domainId}/webhook-urls`, {
                method: 'GET',
            })
            return res as { inbound: string; bounces: string }
        },
        onSuccess: setUrls,
    })

    const copyUrl = async (url: string, label: string) => {
        if (typeof navigator !== 'undefined' && navigator.clipboard) {
            await navigator.clipboard.writeText(url)
            setCopied(label)
            setTimeout(() => setCopied(null), 2000)
        }
    }

    if (!urls) {
        return (
            <Pressable onPress={() => fetchUrls.mutate()} disabled={fetchUrls.isPending}>
                <Text
                    className="text-muted-foreground"
                    style={{ fontSize: 12, textDecorationLine: 'underline' }}
                >
                    {fetchUrls.isPending ? 'Loading...' : 'Show webhook URLs'}
                </Text>
            </Pressable>
        )
    }

    return (
        <View className="gap-2">
            <Text className="text-foreground" style={{ fontSize: 12, fontWeight: '600' }}>
                Webhook URLs
            </Text>
            <WebhookURLRow
                label="Inbound"
                url={urls.inbound}
                isCopied={copied === 'inbound'}
                onCopy={() => copyUrl(urls.inbound, 'inbound')}
                colors={{ mutedColor, foregroundColor, borderColor, surfaceColor }}
            />
            <WebhookURLRow
                label="Bounces"
                url={urls.bounces}
                isCopied={copied === 'bounces'}
                onCopy={() => copyUrl(urls.bounces, 'bounces')}
                colors={{ mutedColor, foregroundColor, borderColor, surfaceColor }}
            />
        </View>
    )
}

function WebhookURLRow({
    label,
    url,
    isCopied,
    onCopy,
    colors,
}: {
    label: string
    url: string
    isCopied: boolean
    onCopy: () => void
    colors: {
        mutedColor: string
        foregroundColor: string
        borderColor: string
        surfaceColor: string
    }
}) {
    return (
        <View className="gap-0.5">
            <Text style={{ fontSize: 11, color: colors.mutedColor }}>{label}</Text>
            <View
                className="flex-row items-center gap-2 px-2 py-1.5 rounded-md border"
                style={{ backgroundColor: colors.surfaceColor, borderColor: colors.borderColor }}
            >
                <Text
                    style={{
                        fontSize: 11,
                        fontFamily: 'monospace',
                        color: colors.foregroundColor,
                        flex: 1,
                    }}
                    numberOfLines={1}
                >
                    {url}
                </Text>
                <Pressable onPress={onCopy} hitSlop={4}>
                    {isCopied ? (
                        <CheckCircle size={14} color={colors.mutedColor} />
                    ) : (
                        <Copy size={14} color={colors.mutedColor} />
                    )}
                </Pressable>
            </View>
        </View>
    )
}

function VerifyButton({ isPending, onPress }: { isPending: boolean; onPress: () => void }) {
    const primaryFgColor = useThemeColor('primary-foreground')
    const Icon = isPending ? Loader2 : RefreshCw
    return (
        <Pressable
            onPress={isPending ? undefined : onPress}
            disabled={isPending}
            className={`flex-row items-center gap-1 px-3 rounded-md py-1.5 bg-primary ${isPending ? 'opacity-60' : 'opacity-100'}`}
        >
            <Icon size={14} color={primaryFgColor} />
            <Text className="text-primary-foreground" style={{ fontSize: 12, fontWeight: '600' }}>
                {isPending ? 'Verifying…' : 'Verify'}
            </Text>
        </Pressable>
    )
}

function VerifyErrorBanner({ message }: { message: string | null }) {
    if (!message) return null
    return (
        <Text className="text-danger" style={{ fontSize: 11 }}>
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

function buildMXHint(
    verified: boolean,
    details: MailDomainVerificationDetails | null,
    provider: 'postmark' | 'smtp'
): string {
    const expected =
        details?.mx?.expected ||
        (provider === 'smtp' ? 'your TinyCld host' : 'inbound.postmarkapp.com')
    if (!expected) {
        return 'no MX required (using IMAP fetch)'
    }
    if (verified) return `MX points to ${expected}`
    const actual = details?.mx?.actual?.join(', ') || details?.mx?.error || 'no MX records'
    return `expected ${expected} — found: ${actual}`
}

function buildProviderHint(
    verified: boolean,
    details: MailDomainVerificationDetails | null,
    provider: 'postmark' | 'smtp'
): string {
    const p = details?.provider
    if (p?.error) return p.error
    if (provider === 'smtp') {
        if (verified) {
            return p?.server_domain
                ? `Public hostname: ${p.server_domain}`
                : 'Public hostname configured'
        }
        return 'Set the SMTP provider public hostname in settings'
    }
    if (verified) {
        return p?.server_domain
            ? `Postmark server InboundDomain: ${p.server_domain}`
            : 'Postmark server InboundDomain matches this domain'
    }
    if (p?.server_domain) {
        return `Postmark server InboundDomain is "${p.server_domain}" — set it to this domain`
    }
    return 'Set this domain as the server InboundDomain in Postmark'
}

function buildOutboundHint(details: MailDomainVerificationDetails | null): string {
    return details?.outbound?.error || 'outbound sending'
}

function DomainVerificationPanel({
    domain,
    provider,
}: {
    domain: DomainRow
    provider: 'postmark' | 'smtp'
}) {
    const details = domain.verification_details
    const outboundHint = buildOutboundHint(details)
    const providerLabel = provider === 'smtp' ? 'SMTP Hostname' : 'Postmark Inbound Domain'

    return (
        <View className="gap-1">
            <CheckRow
                label="Inbound MX"
                ok={domain.mx_verified}
                hint={buildMXHint(domain.mx_verified, details, provider)}
            />
            <CheckRow
                label={providerLabel}
                ok={domain.inbound_domain_verified}
                hint={buildProviderHint(domain.inbound_domain_verified, details, provider)}
            />
            <CheckRow label="SPF" ok={domain.spf_verified} hint={outboundHint} advisory />
            <CheckRow label="DKIM" ok={domain.dkim_verified} hint={outboundHint} advisory />
            <CheckRow
                label="Return-Path"
                ok={domain.return_path_verified}
                hint={outboundHint}
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
    const successColor = useThemeColor('success')
    const dangerColor = useThemeColor('danger')
    const Icon = ok ? CheckCircle : XCircle
    const iconColor = ok ? successColor : advisory ? mutedColor : dangerColor
    return (
        <View className="flex-row gap-2 items-start">
            <Icon size={14} color={iconColor} style={{ marginTop: 2 }} />
            <View className="flex-1">
                <Text className="text-foreground" style={{ fontSize: 12 }}>
                    {label}
                    {advisory ? (
                        <Text className="text-muted-foreground" style={{ fontSize: 10 }}>
                            {' '}
                            (optional)
                        </Text>
                    ) : null}
                </Text>
                <Text className="text-muted-foreground" style={{ fontSize: 11 }}>
                    {hint}
                </Text>
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
    const dangerColor = useThemeColor('danger')

    if (confirming) {
        return (
            <View className="flex-row gap-2">
                <Pressable
                    onPress={onConfirm}
                    className="px-3 rounded-md bg-danger"
                    style={{
                        paddingVertical: 6,
                    }}
                >
                    <Text className="text-danger-foreground" style={{ fontSize: 13 }}>
                        Remove
                    </Text>
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
            className={`flex-row items-center gap-1 px-4 rounded-lg py-2.5 bg-primary ${canSubmit ? 'opacity-100' : 'opacity-50'}`}
        >
            <Plus size={16} color={primaryFgColor} />
            <Text className="text-primary-foreground" style={{ fontWeight: '600' }}>
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
