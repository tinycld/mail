import { useMailHost } from '@tinycld/core/lib/use-mail-host'
import { useState } from 'react'
import { Pressable, Text, View } from 'react-native'
import { DnsRecordRow } from './DnsRecordsPanel'
import { buildMailAppRecords } from './mail-app-records'

/** "DNS records for mail apps" on a verified domain: optional, never part of verification. */
export function MailAppRecordsPanel({
    domainName,
    isVisible,
}: {
    domainName: string
    isVisible: boolean
}) {
    const mailHost = useMailHost()
    const [isOpen, setIsOpen] = useState(false)
    const records = buildMailAppRecords(domainName, mailHost ?? '')
    const arrow = isOpen ? '▾' : '▸'
    const header = `DNS records for mail apps (optional) ${arrow}`
    if (!isVisible || records.length === 0) return null
    return (
        <View className="gap-2">
            <Pressable onPress={() => setIsOpen(open => !open)} accessibilityRole="button">
                <Text className="text-foreground" style={{ fontSize: 12, fontWeight: '600' }}>
                    {header}
                </Text>
            </Pressable>
            <MailAppRecordList isVisible={isOpen} records={records} />
        </View>
    )
}

function MailAppRecordList({
    isVisible,
    records,
}: {
    isVisible: boolean
    records: ReturnType<typeof buildMailAppRecords>
}) {
    if (!isVisible) return null
    const rows = records.map(record => <DnsRecordRow key={record.host} record={record} />)
    return (
        <View className="gap-2">
            <Text className="text-muted-foreground" style={{ fontSize: 11 }}>
                With these records, mail apps find their settings when a person types an address on
                this domain. They do not change verification.
            </Text>
            {rows}
        </View>
    )
}
