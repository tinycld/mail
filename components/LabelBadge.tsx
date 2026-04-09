import { StyleSheet, Text, View } from 'react-native'

interface LabelBadgeProps {
    name: string
    color: string
}

function hexToRgba(hex: string, alpha: number): string {
    const r = Number.parseInt(hex.slice(1, 3), 16)
    const g = Number.parseInt(hex.slice(3, 5), 16)
    const b = Number.parseInt(hex.slice(5, 7), 16)
    return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

export function LabelBadge({ name, color }: LabelBadgeProps) {
    return (
        <View
            style={[
                styles.badge,
                { backgroundColor: hexToRgba(color, 0.12), borderColor: hexToRgba(color, 0.25) },
            ]}
        >
            <Text style={[styles.text, { color }]}>{name}</Text>
        </View>
    )
}

const styles = StyleSheet.create({
    badge: {
        paddingHorizontal: 6,
        paddingVertical: 1,
        borderRadius: 4,
        borderWidth: 1,
    },
    text: {
        fontSize: 11,
        fontWeight: '500',
    },
})
