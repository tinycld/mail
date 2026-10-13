import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import { Trash2 } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, Text, View } from 'react-native'
import { domainRemovalWarning } from './domain-removal'

// Removing is the only way out of a mistyped domain: its DNS records can only
// be published by whoever controls the name, so it can never verify.
export function useRemoveDomain(domainId: string, onRemoved?: () => void) {
    const [domainsCollection] = useStore('mail_domains')
    const [isConfirming, setIsConfirming] = useState(false)
    const remove = useMutation({
        mutationFn: mutation(function* () {
            yield domainsCollection.delete(domainId)
        }),
        onSuccess: () => {
            setIsConfirming(false)
            onRemoved?.()
        },
    })
    return {
        isConfirming,
        onStart: () => setIsConfirming(true),
        onCancel: () => setIsConfirming(false),
        onConfirm: () => remove.mutate(),
    }
}

export function DeleteDomainButton({
    isVisible,
    onPress,
}: {
    isVisible: boolean
    onPress: () => void
}) {
    const dangerColor = useThemeColor('danger')
    if (!isVisible) return null
    return (
        <Pressable className="p-1" onPress={onPress} accessibilityLabel="Remove domain">
            <Trash2 size={16} color={dangerColor} />
        </Pressable>
    )
}

export function RemoveDomainConfirm({
    isVisible,
    domain,
    onConfirm,
    onCancel,
}: {
    isVisible: boolean
    domain: string
    onConfirm: () => void
    onCancel: () => void
}) {
    if (!isVisible) return null
    return (
        <View testID="remove-domain-confirm" className="gap-2 rounded-md border border-danger p-3">
            <Text className="text-foreground" style={{ fontSize: 13 }}>
                {domainRemovalWarning(domain)}
            </Text>
            <View className="flex-row gap-2">
                <ServerActionButton size="sm" variant="destructive" onPress={onConfirm}>
                    <ButtonText>Remove domain</ButtonText>
                </ServerActionButton>
                <Button size="sm" variant="outline" onPress={onCancel}>
                    <ButtonText>Cancel</ButtonText>
                </Button>
            </View>
        </View>
    )
}
