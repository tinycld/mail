import type { OutboundCheckResult } from '@tinycld/app-generated/mail-api'
import { useMutation } from '@tinycld/core/lib/mutations'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import * as Clipboard from 'expo-clipboard'
import { Check, Copy } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, Text, View } from 'react-native'

export type DnsRecord = {
    label: string
    type: 'TXT' | 'CNAME'
    host: string
    value: string
    verified: boolean
}

// buildDnsRecords turns the provider's reported outbound check into the DNS
// rows an admin must publish. Exported for unit test: this mapping is the
// part worth pinning, not the JSX. A self-hosted SMTP deployment can report
// DKIM without a return-path CNAME (or nothing at all) — each record is only
// included when the provider reported both the host and the value it needs.
export function buildDnsRecords(outbound?: OutboundCheckResult): DnsRecord[] {
    if (!outbound) return []
    const records: DnsRecord[] = []
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
                <DnsRecordRow key={record.host} record={record} />
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
