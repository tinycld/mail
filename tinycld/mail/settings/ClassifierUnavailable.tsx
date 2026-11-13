import { Text } from 'react-native'

export function ClassifierUnavailable({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Text testID="mail-classifier-unavailable" className="text-muted-foreground text-sm">
            Ask your admin to turn on AI classification.
        </Text>
    )
}
