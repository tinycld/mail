import { pb } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Download, File, FileArchive, FileImage, FileSpreadsheet, FileText } from 'lucide-react-native'
import { Platform, Pressable, Text, View } from 'react-native'

interface AttachmentThumbnailProps {
    collectionId: string
    recordId: string
    filename: string
    onPress?: () => void
}

export function AttachmentThumbnail({ collectionId, recordId, filename, onPress }: AttachmentThumbnailProps) {
    const mutedColor = useThemeColor('muted-foreground')
    const url = pb.files.getURL({ collectionId, id: recordId }, filename)
    const isImage = /\.(jpe?g|png|gif|webp|svg|bmp)$/i.test(filename)
    const displayName = cleanFilename(filename)
    const Icon = getFileIcon(filename)

    const handlePress = onPress ?? (() => {})

    return (
        <Pressable
            className="rounded-lg border border-border overflow-hidden bg-surface-secondary"
            style={{ width: 160 }}
            onPress={handlePress}
        >
            {isImage ? (
                <ImagePreview url={url} mutedColor={mutedColor} />
            ) : (
                <View
                    className="w-full items-center justify-center bg-surface-secondary"
                    style={{ height: 90 }}
                >
                    <Icon size={24} color={mutedColor} />
                </View>
            )}
            <View className="flex-row items-center px-2 gap-1" style={{ paddingVertical: 6 }}>
                <Text
                    className="flex-1 text-foreground"
                    style={{ fontSize: 12, fontWeight: '500' }}
                    numberOfLines={1}
                >
                    {displayName}
                </Text>
                <View className="flex-row gap-1">
                    <Download size={14} color={mutedColor} />
                </View>
            </View>
        </Pressable>
    )
}

function ImagePreview({ url, mutedColor }: { url: string; mutedColor: string }) {
    if (Platform.OS !== 'web') {
        return (
            <View
                className="w-full items-center justify-center bg-surface-secondary"
                style={{ height: 90 }}
            >
                <FileImage size={24} color={mutedColor} />
            </View>
        )
    }

    return (
        <View className="w-full overflow-hidden" style={{ height: 90 }}>
            <img
                src={url}
                alt=""
                style={{
                    width: '100%',
                    height: '100%',
                    objectFit: 'cover',
                    borderTopLeftRadius: 7,
                    borderTopRightRadius: 7,
                }}
            />
        </View>
    )
}

function getFileIcon(filename: string) {
    const ext = filename.split('.').pop()?.toLowerCase() ?? ''

    if (/^(jpe?g|png|gif|webp|svg|bmp|ico)$/.test(ext)) return FileImage
    if (/^(pdf|doc|docx|txt|rtf|odt|md)$/.test(ext)) return FileText
    if (/^(zip|rar|7z|tar|gz|bz2)$/.test(ext)) return FileArchive
    if (/^(xls|xlsx|csv|ods)$/.test(ext)) return FileSpreadsheet

    return File
}

// PocketBase stores files as {name}_{random10}.{ext} — strip the random suffix for display
function cleanFilename(filename: string) {
    const match = filename.match(/^(.+)_[a-zA-Z0-9]{10}(\.\w+)$/)
    return match ? match[1] + match[2] : filename
}
