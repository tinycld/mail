import {
    Download,
    File,
    FileArchive,
    FileImage,
    FileSpreadsheet,
    FileText,
} from 'lucide-react-native'
import { Linking, Platform, Pressable, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import { pb } from '~/lib/pocketbase'

interface EmailAttachmentsProps {
    isVisible: boolean
    collectionId: string
    recordId: string
    filenames: string[]
}

export function EmailAttachments({
    isVisible,
    collectionId,
    recordId,
    filenames,
}: EmailAttachmentsProps) {
    const theme = useTheme()

    if (!isVisible) return null

    return (
        <View style={[styles.container, { borderTopColor: theme.borderColor.val }]}>
            <Text style={[styles.heading, { color: theme.color8.val }]}>
                {filenames.length} attachment{filenames.length !== 1 ? 's' : ''}
            </Text>
            <View style={styles.grid}>
                {filenames.map(filename => (
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
    const theme = useTheme()
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
            style={[
                styles.thumbnail,
                {
                    borderColor: theme.borderColor.val,
                    backgroundColor: theme.color2.val,
                },
            ]}
            onPress={handlePress}
        >
            {isImage ? (
                <ImagePreview url={url} theme={theme} />
            ) : (
                <View style={[styles.iconPreview, { backgroundColor: theme.color3.val }]}>
                    <Icon size={24} color={theme.color8.val} />
                </View>
            )}
            <View style={styles.info}>
                <Text style={[styles.filename, { color: theme.color.val }]} numberOfLines={1}>
                    {displayName}
                </Text>
                <View style={styles.actions}>
                    <Download size={14} color={theme.color8.val} />
                </View>
            </View>
        </Pressable>
    )
}

function ImagePreview({ url, theme }: { url: string; theme: ReturnType<typeof useTheme> }) {
    if (Platform.OS !== 'web') {
        return (
            <View style={[styles.iconPreview, { backgroundColor: theme.color3.val }]}>
                <FileImage size={24} color={theme.color8.val} />
            </View>
        )
    }

    return (
        <View style={styles.imageContainer}>
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

const styles = StyleSheet.create({
    container: {
        paddingHorizontal: 16,
        paddingVertical: 12,
        borderTopWidth: 1,
        gap: 8,
    },
    heading: {
        fontSize: 12,
        fontWeight: '600',
        textTransform: 'uppercase',
        letterSpacing: 0.5,
    },
    grid: {
        flexDirection: 'row',
        flexWrap: 'wrap',
        gap: 8,
    },
    thumbnail: {
        width: 160,
        borderRadius: 8,
        borderWidth: 1,
        overflow: 'hidden',
    },
    imageContainer: {
        width: '100%',
        height: 90,
        overflow: 'hidden',
    },
    iconPreview: {
        width: '100%',
        height: 90,
        alignItems: 'center',
        justifyContent: 'center',
    },
    info: {
        flexDirection: 'row',
        alignItems: 'center',
        paddingHorizontal: 8,
        paddingVertical: 6,
        gap: 4,
    },
    filename: {
        fontSize: 12,
        fontWeight: '500',
        flex: 1,
    },
    actions: {
        flexDirection: 'row',
        gap: 4,
    },
})
