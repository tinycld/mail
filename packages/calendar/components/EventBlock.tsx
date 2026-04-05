import { Pressable, StyleSheet, Text, View } from 'react-native'

interface EventBlockProps {
    title: string
    timeLabel: string
    bgColor: string
    textColor: string
    topOffset: number
    height: number
    left?: number
    width?: number
    onPress: () => void
}

export function EventBlock({
    title,
    timeLabel,
    bgColor,
    textColor,
    topOffset,
    height,
    left = 0,
    width = 100,
    onPress,
}: EventBlockProps) {
    const showTwoLines = height > 40

    return (
        <Pressable
            onPress={onPress}
            style={{
                position: 'absolute',
                top: topOffset,
                left: `${left}%`,
                width: `${width}%`,
                height: Math.max(height - 2, 18),
                paddingHorizontal: 1,
                zIndex: 5,
            }}
        >
            <View style={[styles.block, { backgroundColor: bgColor }]}>
                <Text style={[styles.title, { color: textColor }]} numberOfLines={1}>
                    {title}
                </Text>
                {showTwoLines && (
                    <Text style={[styles.time, { color: textColor }]} numberOfLines={1}>
                        {timeLabel}
                    </Text>
                )}
            </View>
        </Pressable>
    )
}

const styles = StyleSheet.create({
    block: {
        flex: 1,
        borderRadius: 4,
        paddingHorizontal: 6,
        paddingVertical: 2,
        overflow: 'hidden',
    },
    title: {
        fontSize: 12,
        fontWeight: '600',
    },
    time: {
        fontSize: 11,
        opacity: 0.9,
    },
})
