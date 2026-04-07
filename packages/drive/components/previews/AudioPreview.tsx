import { Platform, StyleSheet, View } from 'react-native'
import { SizableText, useTheme } from 'tamagui'
import { getFileURL } from '../../lib/file-url'
import type { PreviewProps } from '../../lib/preview-registry'
import { getFileIcon } from '../file-icons'
import { GenericPreview } from './GenericPreview'

export function AudioPreview(props: PreviewProps) {
    const { item } = props
    const theme = useTheme()
    const { icon: FileIcon, color: iconColor } = getFileIcon(item.category, theme.color8.val)
    const fileUrl = getFileURL(item)

    if (!fileUrl) return null

    if (Platform.OS === 'web') {
        return (
            <View style={styles.container}>
                <FileIcon size={80} color={iconColor} />
                <SizableText size="$5" fontWeight="600" color="$color" marginTop="$4">
                    {item.name}
                </SizableText>
                <View style={styles.audioWrapper}>
                    {/* biome-ignore lint/a11y/useMediaCaption: captions not available for user uploads */}
                    <audio src={fileUrl} controls style={{ width: '100%' }} />
                </View>
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
        padding: 32,
    },
    audioWrapper: {
        width: '100%',
        maxWidth: 400,
        marginTop: 24,
    },
})
