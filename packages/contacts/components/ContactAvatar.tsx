import { StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'

const AVATAR_COLORS = [
    '$blue8',
    '$green8',
    '$purple8',
    '$orange8',
    '$pink8',
    '$red8',
    '$yellow8',
    '$cyan8',
] as const

function hashName(name: string): number {
    let hash = 0
    for (let i = 0; i < name.length; i++) {
        hash = (hash << 5) - hash + name.charCodeAt(i)
        hash |= 0
    }
    return Math.abs(hash)
}

interface ContactAvatarProps {
    firstName: string
    lastName?: string
    size?: number
}

export function ContactAvatar({ firstName, lastName, size = 40 }: ContactAvatarProps) {
    const theme = useTheme()
    const fullName = `${firstName} ${lastName ?? ''}`.trim()
    const colorToken = AVATAR_COLORS[hashName(fullName) % AVATAR_COLORS.length]
    const backgroundColor = theme[colorToken.replace('$', '') as keyof typeof theme]?.val ?? theme.blue8.val
    const letter = (firstName[0] ?? '?').toUpperCase()
    const fontSize = size * 0.42

    return (
        <View style={[styles.container, { width: size, height: size, borderRadius: size / 2, backgroundColor }]}>
            <Text style={[styles.letter, { fontSize, lineHeight: size }]}>
                {letter}
            </Text>
        </View>
    )
}

const styles = StyleSheet.create({
    container: {
        alignItems: 'center',
        justifyContent: 'center',
    },
    letter: {
        color: '#fff',
        fontWeight: '600',
        textAlign: 'center',
    },
})
