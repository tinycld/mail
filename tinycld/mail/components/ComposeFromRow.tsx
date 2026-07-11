import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Text, View } from 'react-native'
import { useSendableIdentities } from '../hooks/useSendableIdentities'
import { useComposeStore } from '../stores/compose-store'
import { FromIdentityPicker } from './FromIdentityPicker'

export function ComposeFromRow() {
    const identities = useSendableIdentities()
    const mailboxId = useComposeStore(s => s.mailboxId)
    const aliasId = useComposeStore(s => s.aliasId)
    const setFromIdentity = useComposeStore(s => s.setFromIdentity)

    const mutedColor = useThemeColor('muted-foreground')
    const fgColor = useThemeColor('foreground')
    const borderColor = useThemeColor('border')

    if (identities.length === 0) return null

    // Resolve the displayed identity during render rather than syncing a
    // default into the store via an effect: an unset store mailboxId (a fresh
    // compose) falls back to the first identity here, and the send paths read
    // their own mailbox (useMailSendReadiness / useDefaultMailbox), so no store
    // write is needed until the user explicitly picks a different From.
    const resolved = identities.find(i => i.mailboxId === mailboxId) ?? identities[0]
    const currentAddress = aliasId
        ? (resolved.aliases.find(a => a.id === aliasId)?.address ?? resolved.primaryAddress)
        : resolved.primaryAddress

    const hasChoices = identities.length > 1 || identities.some(i => i.aliases.length > 0)

    return (
        <View
            className="flex-row items-center px-3 py-1"
            style={{ minHeight: 36, borderBottomWidth: 1, borderBottomColor: borderColor }}
        >
            <Text style={{ fontSize: 13, width: 56, color: mutedColor }}>From</Text>
            <Text className="flex-1" style={{ fontSize: 13, color: fgColor }}>
                {resolved.mailboxDisplayName} &lt;{currentAddress}&gt;
            </Text>
            {hasChoices && (
                <FromIdentityPicker
                    identities={identities}
                    selectedMailboxId={resolved.mailboxId}
                    selectedAliasId={aliasId}
                    onSelect={setFromIdentity}
                />
            )}
        </View>
    )
}
