import { Image, StyleSheet, View } from 'react-native'
import { useTheme } from 'tamagui'
import { getThumbnailURL } from '../lib/file-url'
import type { DriveItemView } from '../types'
import { getFileIcon } from './file-icons'

interface ThumbnailProps {
    item: DriveItemView
    size?: number
}

export function Thumbnail({ item, size = 120 }: ThumbnailProps) {
    const theme = useTheme()
    const { icon: FileIcon, color: iconColor } = getFileIcon(item.category, theme.color8.val)

    const thumbnailUrl = getThumbnailURL(item, `${size}x${size}`)

    if (!thumbnailUrl) {
        return (
            <View style={[styles.iconContainer, { height: size }]}>
                <FileIcon size={size * 0.33} color={iconColor} />
            </View>
        )
    }

    return (
        <Image
            source={{ uri: thumbnailUrl }}
            style={[styles.image, { width: size, height: size }]}
            resizeMode="cover"
        />
    )
}

const styles = StyleSheet.create({
    iconContainer: {
        alignItems: 'center',
        justifyContent: 'center',
        width: '100%',
    },
    image: {
        borderRadius: 4,
    },
})
