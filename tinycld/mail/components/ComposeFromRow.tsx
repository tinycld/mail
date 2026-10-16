import type { ComponentProps } from 'react'
import { Text, View } from 'react-native'
import type { SendableIdentity } from '../hooks/groupSendableIdentities'
import { useSendableIdentities } from '../hooks/useSendableIdentities'
import { useComposeStore } from '../stores/compose-store'
import { FromIdentityPicker } from './FromIdentityPicker'

// Resolve the shown From during render instead of syncing a default into the
// store: an empty store mailboxId means the personal mailbox (identities[0]),
// and a null address means that mailbox's default. Nothing is written until
// the user picks.
function resolveFrom(identities: SendableIdentity[], mailboxId: string, addressId: string | null) {
    const identity = identities.find(i => i.mailboxId === mailboxId) ?? identities[0]
    const address =
        identity.addresses.find(a => a.id === addressId) ??
        identity.addresses.find(a => a.id === identity.defaultAddressId) ??
        identity.addresses[0]
    return { identity, address }
}

export function ComposeFromRow() {
    const identities = useSendableIdentities()
    const mailboxId = useComposeStore(s => s.mailboxId)
    const fromAddressId = useComposeStore(s => s.fromAddressId)
    const setFrom = useComposeStore(s => s.setFrom)

    if (identities.length === 0) return null

    const { identity, address } = resolveFrom(identities, mailboxId, fromAddressId)
    const hasChoices = identities.length > 1 || identity.addresses.length > 1
    const label = `${identity.mailboxDisplayName} <${address.email}>`

    return (
        <View
            className="flex-row items-center px-3 py-1 border-b border-border"
            style={{ minHeight: 36 }}
        >
            <Text className="text-muted-foreground" style={{ fontSize: 13, width: 56 }}>
                From
            </Text>
            <Text className="flex-1 text-foreground" style={{ fontSize: 13 }}>
                {label}
            </Text>
            <PickerSlot
                isVisible={hasChoices}
                identities={identities}
                selectedAddressId={address.id}
                onSelect={setFrom}
            />
        </View>
    )
}

function PickerSlot({
    isVisible,
    ...props
}: { isVisible: boolean } & ComponentProps<typeof FromIdentityPicker>) {
    if (!isVisible) return null
    return <FromIdentityPicker {...props} />
}
