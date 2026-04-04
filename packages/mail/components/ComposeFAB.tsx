import { Pencil } from 'lucide-react-native'
import { Pressable, StyleSheet } from 'react-native'
import { useTheme } from 'tamagui'
import { composeEvents } from '../hooks/composeEvents'

interface ComposeFABProps {
    isVisible: boolean
}

export function ComposeFAB({ isVisible }: ComposeFABProps) {
    const theme = useTheme()

    if (!isVisible) return null

    return (
        <Pressable
            style={[styles.fab, { backgroundColor: theme.accentBackground.val }]}
            onPress={() => composeEvents.emit()}
            accessibilityLabel="Compose email"
        >
            <Pencil size={22} color={theme.accentColor.val} />
        </Pressable>
    )
}

const styles = StyleSheet.create({
    fab: {
        position: 'absolute',
        bottom: 80,
        right: 16,
        width: 56,
        height: 56,
        borderRadius: 28,
        alignItems: 'center',
        justifyContent: 'center',
        elevation: 4,
        shadowColor: '#000',
        shadowOffset: { width: 0, height: 2 },
        shadowOpacity: 0.25,
        shadowRadius: 4,
        zIndex: 50,
    },
})
