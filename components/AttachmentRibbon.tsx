import { X } from 'lucide-react-native'
import { Pressable, ScrollView, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import { formatBytes } from '~/lib/format-utils'
import type { AttachmentFile } from '../hooks/useAttachments'

interface AttachmentRibbonProps {
    isVisible: boolean
    attachments: AttachmentFile[]
    onRemove: (id: string) => void
}

export function AttachmentRibbon({ isVisible, attachments, onRemove }: AttachmentRibbonProps) {
    const theme = useTheme()

    if (!isVisible) return null

    return (
        <View style={[styles.container, { borderTopColor: theme.borderColor.val }]}>
            <ScrollView horizontal showsHorizontalScrollIndicator={false} style={styles.scroll}>
                <View style={styles.chipRow}>
                    {attachments.map(att => (
                        <AttachmentChip
                            key={att.id}
                            attachment={att}
                            onRemove={() => onRemove(att.id)}
                        />
                    ))}
                </View>
            </ScrollView>
        </View>
    )
}

function AttachmentChip({
    attachment,
    onRemove,
}: {
    attachment: AttachmentFile
    onRemove: () => void
}) {
    const theme = useTheme()

    const truncatedName =
        attachment.name.length > 24 ? `${attachment.name.slice(0, 21)}...` : attachment.name

    return (
        <View
            style={[
                styles.chip,
                {
                    backgroundColor: theme.color3.val,
                    borderColor: theme.borderColor.val,
                },
            ]}
        >
            <Text style={[styles.chipName, { color: theme.color.val }]} numberOfLines={1}>
                {truncatedName}
            </Text>
            <Text style={[styles.chipSize, { color: theme.color8.val }]}>
                {formatBytes(attachment.size)}
            </Text>
            <Pressable onPress={onRemove} style={styles.chipRemove} hitSlop={4}>
                <X size={12} color={theme.color8.val} />
            </Pressable>
        </View>
    )
}

const styles = StyleSheet.create({
    container: {
        borderTopWidth: 1,
        paddingVertical: 6,
        paddingHorizontal: 12,
    },
    scroll: {
        flexGrow: 0,
    },
    chipRow: {
        flexDirection: 'row',
        gap: 6,
    },
    chip: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 4,
        paddingHorizontal: 8,
        paddingVertical: 4,
        borderRadius: 6,
        borderWidth: 1,
    },
    chipName: {
        fontSize: 12,
        fontWeight: '500',
        maxWidth: 160,
    },
    chipSize: {
        fontSize: 11,
    },
    chipRemove: {
        padding: 2,
    },
})
