import { eq } from '@tanstack/db'
import { X } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import { AddAliasForm } from './AddAliasForm'

interface Props {
    mailboxId: string
    mailboxDomainId: string
    domainName: string
}

export function MailboxAliasesPanel({ mailboxId, mailboxDomainId, domainName }: Props) {
    const mutedColor = useThemeColor('muted-foreground')
    const fgColor = useThemeColor('foreground')
    const dangerColor = useThemeColor('danger')
    const borderColor = useThemeColor('border')

    const [aliasesCollection] = useStore('mail_mailbox_aliases')

    const { data: aliases } = useOrgLiveQuery(
        (query) =>
            query
                .from({ mail_mailbox_aliases: aliasesCollection })
                .where(({ mail_mailbox_aliases }) => eq(mail_mailbox_aliases.mailbox, mailboxId)),
        [mailboxId]
    )

    const remove = useMutation({
        mutationFn: mutation(function* (aliasId: string) {
            yield aliasesCollection.delete(aliasId)
        }),
    })

    const items = aliases ?? []

    return (
        <View className="gap-2 mt-3 pt-3" style={{ borderTopWidth: 1, borderColor }}>
            <Text style={{ fontSize: 13, fontWeight: '600', color: fgColor }}>Aliases</Text>

            {items.length === 0 && (
                <Text style={{ fontSize: 12, color: mutedColor }}>No aliases yet.</Text>
            )}

            {items.map((alias) => (
                <View key={alias.id} className="flex-row items-center justify-between py-1">
                    <Text style={{ fontSize: 13, color: fgColor }}>
                        {alias.address}@{domainName}
                    </Text>
                    <Pressable onPress={() => remove.mutate(alias.id)} className="p-1">
                        <X size={14} color={dangerColor} />
                    </Pressable>
                </View>
            ))}

            <AddAliasForm
                mailboxId={mailboxId}
                mailboxDomainId={mailboxDomainId}
                domainName={domainName}
            />
        </View>
    )
}
