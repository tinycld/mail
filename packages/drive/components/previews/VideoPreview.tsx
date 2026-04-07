import { Platform, StyleSheet, View } from 'react-native'
import { getFileURL } from '../../lib/file-url'
import type { PreviewProps } from '../../lib/preview-registry'
import { GenericPreview } from './GenericPreview'

export function VideoPreview(props: PreviewProps) {
    const { item } = props
    const fileUrl = getFileURL(item)

    if (!fileUrl) return null

    if (Platform.OS === 'web') {
        return (
            <View style={styles.container}>
                {/* biome-ignore lint/a11y/useMediaCaption: captions not available for user uploads */}
                <video src={fileUrl} controls style={{ maxWidth: '100%', maxHeight: '100%' }} />
            </View>
        )
    }

    return <GenericPreview {...props} />
}

const styles = StyleSheet.create({
    container: {
        flex: 1,
        alignItems: 'center',
        justifyContent: 'center',
    },
})
