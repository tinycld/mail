import type { OutboundCheckResult } from '@tinycld/app-generated/mail-api'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Check } from 'lucide-react-native'
import { Text, View } from 'react-native'
import { MONOSPACE_FONT } from '../lib/monospace-font'
import { CopyButton } from './CopyButton'

export type DnsRecord = {
    label: string
    type: 'MX' | 'TXT' | 'CNAME' | 'SRV'
    host: string
    value: string
    // null: not checked yet — a row whose stored details predate the flag.
    verified: boolean | null
    note?: string
}

// buildDnsRecords turns the provider's reported outbound check into the DNS
// rows an admin must publish. Exported for unit test: this mapping is the
// part worth pinning, not the JSX. A self-hosted SMTP deployment can report
// DKIM without a return-path CNAME (or nothing at all) — each record is only
// included when the provider reported both the host and the value it needs.
//
// MX leads the list (inbound routing). Its verified flag is the last MX
// check's result. Details stored before mx_verified existed lack it, so it
// reads as unknown (no badge, not "left to publish") until the next Verify.
export function buildDnsRecords(outbound?: OutboundCheckResult): DnsRecord[] {
    if (!outbound) return []
    const records: DnsRecord[] = []
    if (outbound.mx_host) {
        records.push({
            label: 'MX',
            type: 'MX',
            host: '@',
            value: `10 ${outbound.mx_host}`,
            verified: outbound.mx_verified ?? null,
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
    return buildDnsRecords(outbound).some(record => record.verified === false)
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

export function DnsRecordRow({ record }: { record: DnsRecord }) {
    const successColor = useThemeColor('success')

    return (
        <View className="border border-border rounded p-2 gap-1">
            <View className="flex-row items-center gap-2">
                <Text className="text-foreground" style={{ fontSize: 11, fontWeight: '600' }}>
                    {record.label} ({record.type})
                </Text>
                <VerifiedBadge isVisible={record.verified === true} successColor={successColor} />
            </View>
            <Text className="text-muted-foreground" style={{ fontSize: 11 }}>
                {record.host}
            </Text>
            <View className="flex-row items-center gap-2">
                <Text
                    className="text-foreground flex-1"
                    numberOfLines={1}
                    style={{ fontSize: 11, fontFamily: MONOSPACE_FONT }}
                >
                    {record.value}
                </Text>
                <CopyButton value={record.value} label={`Copy ${record.label} value`} />
            </View>
            <RecordNote note={record.note} />
        </View>
    )
}

function RecordNote({ note }: { note?: string }) {
    if (!note) return null
    return (
        <Text className="text-muted-foreground" style={{ fontSize: 11 }}>
            {note}
        </Text>
    )
}

function VerifiedBadge({ isVisible, successColor }: { isVisible: boolean; successColor: string }) {
    if (!isVisible) return null
    return <Check size={12} color={successColor} />
}
