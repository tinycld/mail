import { Download, File, FileArchive, FileImage, FileSpreadsheet, FileText } from 'lucide-react-native'
import { Linking, Platform, Pressable, Text, View } from 'react-native'
import { pb } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'

interface EmailAttachmentsProps {
    isVisible: boolean
    collectionId: string
    recordId: string
    filenames: string[]
}

export function EmailAttachments({ isVisible, collectionId, recordId, filenames }: EmailAttachmentsProps) {
    const borderColor = useThemeColor('border')
    const mutedColor = useThemeColor('muted-foreground')

    if (!isVisible) return null

    return (
        <View
            className="px-4 py-3 gap-2"
            style={{
                borderTopWidth: 1,
                borderTopColor: borderColor,
            }}
        >
            <Text
                style={{
                    fontSize: 12,
                    fontWeight: '600',
                    textTransform: 'uppercase',
                    letterSpacing: 0.5,
                    color: mutedColor,
                }}
            >
                {filenames.length} attachment{filenames.length !== 1 ? 's' : ''}
            </Text>
            <View className="flex-row flex-wrap gap-2">
                {filenames.map((filename) => (
                    <AttachmentThumbnail
                        key={filename}
                        collectionId={collectionId}
                        recordId={recordId}
                        filename={filename}
                    />
                ))}
            </View>
        </View>
    )
}

function AttachmentThumbnail({
    collectionId,
    recordId,
    filename,
}: {
    collectionId: string
    recordId: string
    filename: string
}) {
    const borderColor = useThemeColor('border')
    const mutedColor = useThemeColor('muted-foreground')
    const foregroundColor = useThemeColor('foreground')
    const surfaceColor = useThemeColor('surface-secondary')
    const url = pb.files.getURL({ collectionId, id: recordId }, filename)
    const isImage = /\.(jpe?g|png|gif|webp|svg|bmp)$/i.test(filename)
    const displayName = cleanFilename(filename)
    const Icon = getFileIcon(filename)

    const handlePress = () => {
        if (Platform.OS === 'web') {
            window.open(url, '_blank')
        } else {
            Linking.openURL(url)
        }
    }

    return (
        <Pressable
            className="rounded-lg border overflow-hidden"
            style={{
                width: 160,
                borderColor,
                backgroundColor: surfaceColor,
            }}
            onPress={handlePress}
        >
            {isImage ? (
                <ImagePreview url={url} mutedColor={mutedColor} surfaceColor={surfaceColor} />
            ) : (
                <View
                    className="w-full items-center justify-center"
                    style={{
                        height: 90,
                        backgroundColor: surfaceColor,
                    }}
                >
                    <Icon size={24} color={mutedColor} />
                </View>
            )}
            <View className="flex-row items-center px-2 gap-1" style={{ paddingVertical: 6 }}>
                <Text
                    className="flex-1"
                    style={{
                        fontSize: 12,
                        fontWeight: '500',
                        color: foregroundColor,
                    }}
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

function ImagePreview({ url, mutedColor, surfaceColor }: { url: string; mutedColor: string; surfaceColor: string }) {
    if (Platform.OS !== 'web') {
        return (
            <View
                className="w-full items-center justify-center"
                style={{
                    height: 90,
                    backgroundColor: surfaceColor,
                }}
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
