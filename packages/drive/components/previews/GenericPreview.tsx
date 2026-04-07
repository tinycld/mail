import { Download } from 'lucide-react-native'
import { StyleSheet, View } from 'react-native'
import { Button, SizableText, useTheme } from 'tamagui'
import { formatBytes } from '~/lib/format-utils'
import { getFileURL } from '../../lib/file-url'
import type { PreviewProps } from '../../lib/preview-registry'
import { getFileIcon } from '../file-icons'

export function GenericPreview({ item }: PreviewProps) {
    const theme = useTheme()
    const { icon: FileIcon, color: iconColor } = getFileIcon(item.category, theme.color8.val)
    const fileUrl = getFileURL(item)

    const handleDownload = () => {
        if (!fileUrl) return
        if (typeof window !== 'undefined') {
            const a = document.createElement('a')
            a.href = fileUrl
            a.download = item.name
            a.click()
        }
    }

    return (
        <View style={styles.container}>
            <FileIcon size={80} color={iconColor} />
            <SizableText size="$5" fontWeight="600" color="$color" marginTop="$4">
                {item.name}
            </SizableText>
            <SizableText size="$3" color="$color10" marginTop="$1">
                {item.mimeType} · {formatBytes(item.size)}
            </SizableText>
            {fileUrl && (
                <Button
                    size="$4"
                    theme="accent"
                    marginTop="$5"
                    onPress={handleDownload}
                    icon={Download}
                >
                    <Button.Text fontWeight="600">Download</Button.Text>
                </Button>
            )}
        </View>
    )
}

const styles = StyleSheet.create({
    container: {
        flex: 1,
        alignItems: 'center',
        justifyContent: 'center',
        padding: 32,
    },
})
