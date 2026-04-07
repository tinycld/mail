import { Platform, StyleSheet, View } from 'react-native'
import { getFileURL } from '../../lib/file-url'
import type { PreviewProps } from '../../lib/preview-registry'
import { GenericPreview } from './GenericPreview'

export function PdfPreview(props: PreviewProps) {
    const { item } = props
    const fileUrl = getFileURL(item)

    if (!fileUrl) return null

    if (Platform.OS === 'web') {
        return (
            <View style={styles.container}>
                <iframe
                    src={fileUrl}
                    title={item.name}
                    style={{ width: '100%', height: '100%', border: 'none' }}
                />
            </View>
        )
    }

    return <GenericPreview {...props} />
}

const styles = StyleSheet.create({
    container: {
        flex: 1,
    },
})
