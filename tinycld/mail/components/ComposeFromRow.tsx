import type { ComponentProps } from 'react'
import { Text, View } from 'react-native'
import { composeFromRowView, type ResolvedFrom } from '../hooks/composeFrom'
import type { SendableIdentity } from '../hooks/groupSendableIdentities'
import { FromIdentityPicker } from './FromIdentityPicker'

interface Props {
    identities: SendableIdentity[]
    // What the parent sends from (useComposeFrom), so the row never shows a
    // different From from the one the payload uses.
    from: ResolvedFrom | null
    onSelect: (mailboxId: string, addressId: string) => void
}

export function ComposeFromRow({ identities, from, onSelect }: Props) {
    const { isVisible, label, hasChoices, selectedAddressId } = composeFromRowView(identities, from)
    if (!isVisible) return null

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
                selectedAddressId={selectedAddressId}
                onSelect={onSelect}
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
