import type { OutboundCheckResult } from '@tinycld/app-generated/mail-api'
import { useMutation } from '@tinycld/core/lib/mutations'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import * as Clipboard from 'expo-clipboard'
import { Check, Copy } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, Text, View } from 'react-native'

export type DnsRecord = {
    label: string
    type: 'MX' | 'TXT' | 'CNAME'
    host: string
    value: string
    verified: boolean
}

// buildDnsRecords turns the provider's reported outbound check into the DNS
// rows an admin must publish. Exported for unit test: this mapping is the
// part worth pinning, not the JSX. A self-hosted SMTP deployment can report
// DKIM without a return-path CNAME (or nothing at all) — each record is only
// included when the provider reported both the host and the value it needs.
//
// MX leads the list (inbound routing) followed by SPF (outbound
// authorization) when the provider has an include to publish — Postmark
// deprecated SPF entirely, so spf_include stays empty on that path and the
// row is omitted rather than shown blank. MX has no per-check verified flag
// on OutboundCheckResult (that's domain.mx_verified, surfaced by its own
// "Inbound MX" CheckRow elsewhere) so it is always rendered "verified" here —
// it must never make hasUnpublishedDnsRecords report the panel as having
// something left to publish.
export function buildDnsRecords(outbound?: OutboundCheckResult): DnsRecord[] {
    if (!outbound) return []
    const records: DnsRecord[] = []
    if (outbound.mx_host) {
        records.push({
            label: 'MX',
            type: 'MX',
            host: '@',
            value: `MX 10 ${outbound.mx_host}`,
            verified: true,
        })
    }
    if (outbound.spf_include) {
        records.push({
            label: 'SPF',
            type: 'TXT',
            host: '@',
            value: `v=spf1 include:${outbound.spf_include} ~all`,
            verified: outbound.spf,
        })
    }
    if (outbound.dkim_host && outbound.dkim_text_value) {
        records.push({
            label: 'DKIM',
            type: 'TXT',
            host: outbound.dkim_host,
            value: outbound.dkim_text_value,
            verified: outbound.dkim,
        })
    }
    if (outbound.return_path_domain && outbound.return_path_cname_value) {
        records.push({
            label: 'Return-Path',
            type: 'CNAME',
            host: outbound.return_path_domain,
            value: outbound.return_path_cname_value,
            verified: outbound.return_path,
        })
    }
    return records
}

// hasUnpublishedDnsRecords reports whether the panel still has something
// actionable to show. This — not the domain's overall verdict — is what gates
// visibility: `verified` deliberately excludes DKIM and return-path (they are
// advisory), so gating on it hid the panel the moment MX and inbound went
// green, taking the host/value/copy UI away while those rows were still red
// and leaving the admin a red row with no way to fix it.
export function hasUnpublishedDnsRecords(outbound?: OutboundCheckResult): boolean {
    return buildDnsRecords(outbound).some(record => !record.verified)
}

export function DnsRecordsPanel({
    outbound,
    isVisible,
}: {
    outbound?: OutboundCheckResult
    isVisible: boolean
}) {
    const records = buildDnsRecords(outbound)
    if (!isVisible || records.length === 0) return null

    return (
        <View className="gap-2 mt-2">
            <Text className="text-foreground" style={{ fontSize: 12, fontWeight: '600' }}>
                DNS records to publish
            </Text>
            {records.map(record => (
                <DnsRecordRow key={record.label} record={record} />
            ))}
        </View>
    )
}

function DnsRecordRow({ record }: { record: DnsRecord }) {
    const mutedColor = useThemeColor('muted-foreground')
    const successColor = useThemeColor('success')
    const [copied, setCopied] = useState(false)

    const copy = useMutation({
        mutationFn: async () => {
            await Clipboard.setStringAsync(record.value)
        },
        onSuccess: () => {
            setCopied(true)
            setTimeout(() => setCopied(false), 1500)
        },
    })

    return (
        <View className="border border-border rounded p-2 gap-1">
            <View className="flex-row items-center gap-2">
                <Text className="text-foreground" style={{ fontSize: 11, fontWeight: '600' }}>
                    {record.label} ({record.type})
                </Text>
                <VerifiedBadge isVisible={record.verified} successColor={successColor} />
            </View>
            <Text className="text-muted-foreground" style={{ fontSize: 11 }}>
                {record.host}
            </Text>
            <View className="flex-row items-center gap-2">
                <Text
                    className="text-foreground flex-1"
                    numberOfLines={1}
                    style={{ fontSize: 11, fontFamily: 'monospace' }}
                >
                    {record.value}
                </Text>
                <Pressable
                    onPress={() => copy.mutate()}
                    accessibilityLabel={`Copy ${record.label} value`}
                >
                    {copied ? (
                        <Check size={14} color={successColor} />
                    ) : (
                        <Copy size={14} color={mutedColor} />
                    )}
                </Pressable>
            </View>
        </View>
    )
}

function VerifiedBadge({ isVisible, successColor }: { isVisible: boolean; successColor: string }) {
    if (!isVisible) return null
    return <Check size={12} color={successColor} />
}
