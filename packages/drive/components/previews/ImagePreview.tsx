import { Image, StyleSheet, View } from 'react-native'
import { getFileURL } from '../../lib/file-url'
import type { PreviewProps } from '../../lib/preview-registry'

export function ImagePreview({ item }: PreviewProps) {
    const fileUrl = getFileURL(item)

    if (!fileUrl) return null

    return (
        <View style={styles.container}>
            <Image source={{ uri: fileUrl }} style={styles.image} resizeMode="contain" />
        </View>
    )
}

const styles = StyleSheet.create({
    container: {
        flex: 1,
        alignItems: 'center',
        justifyContent: 'center',
        padding: 16,
    },
    image: {
        width: '100%',
        height: '100%',
    },
})
