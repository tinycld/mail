import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useState } from 'react'
import { Modal, Pressable, ScrollView, Text, useWindowDimensions, View } from 'react-native'
import type { SendableIdentity } from '../hooks/groupSendableIdentities'

interface Props {
    identities: SendableIdentity[]
    selectedAddressId: string
    onSelect: (mailboxId: string, addressId: string) => void
}

export function FromIdentityPicker({ identities, selectedAddressId, onSelect }: Props) {
    const [isOpen, setIsOpen] = useState(false)
    const primaryColor = useThemeColor('primary')
    const surfaceBg = useThemeColor('surface-secondary')
    const backdropColor = useThemeColor('overlay-backdrop')
    // Many mailboxes or addresses must scroll inside the panel, not run off
    // the screen.
    const { height } = useWindowDimensions()
    const listMaxHeight = Math.min(480, height * 0.7)

    const choose = (mailboxId: string, addressId: string) => {
        onSelect(mailboxId, addressId)
        setIsOpen(false)
    }

    return (
        <>
            <Pressable onPress={() => setIsOpen(true)}>
                <Text
                    style={{ fontSize: 13, color: primaryColor, textDecorationLine: 'underline' }}
                >
                    Change
                </Text>
            </Pressable>

            <Modal transparent visible={isOpen} onRequestClose={() => setIsOpen(false)}>
                <Pressable
                    className="flex-1 items-center justify-center"
                    style={{ backgroundColor: backdropColor }}
                    onPress={() => setIsOpen(false)}
                >
                    <View
                        className="rounded-xl p-3"
                        style={{ backgroundColor: surfaceBg, minWidth: 320 }}
                    >
                        <ScrollView
                            style={{ maxHeight: listMaxHeight }}
                            contentContainerClassName="gap-3"
                        >
                            {identities.map(identity => (
                                <IdentityGroup
                                    key={identity.mailboxId}
                                    identity={identity}
                                    selectedAddressId={selectedAddressId}
                                    onChoose={choose}
                                />
                            ))}
                        </ScrollView>
                    </View>
                </Pressable>
            </Modal>
        </>
    )
}

function IdentityGroup({
    identity,
    selectedAddressId,
    onChoose,
}: {
    identity: SendableIdentity
    selectedAddressId: string
    onChoose: (mailboxId: string, addressId: string) => void
}) {
    return (
        <View className="gap-1">
            <Text
                className="text-muted-foreground uppercase"
                style={{ fontSize: 12, fontWeight: '600' }}
            >
                {identity.mailboxDisplayName}
            </Text>
            {identity.addresses.map(address => (
                <AddressRow
                    key={address.id}
                    label={address.email}
                    isSelected={address.id === selectedAddressId}
                    isUnverified={!address.domainVerified}
                    onPress={() => onChoose(identity.mailboxId, address.id)}
                />
            ))}
        </View>
    )
}

function AddressRow({
    label,
    isSelected,
    isUnverified,
    onPress,
}: {
    label: string
    isSelected: boolean
    isUnverified: boolean
    onPress: () => void
}) {
    const selectedClass = isSelected ? 'bg-primary/10' : ''
    return (
        <Pressable
            onPress={onPress}
            className={`flex-row items-center px-2 py-1.5 rounded-md ${selectedClass}`}
        >
            <Text className="text-foreground" style={{ fontSize: 13 }}>
                {label}
            </Text>
            <UnverifiedNote isVisible={isUnverified} />
        </Pressable>
    )
}

// Mail from an unverified domain fails to send; say so before it is picked.
function UnverifiedNote({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Text className="ml-2 text-muted-foreground" style={{ fontSize: 12 }}>
            domain not verified
        </Text>
    )
}
