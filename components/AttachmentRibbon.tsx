import { useThemeColor } from 'heroui-native'
import { X } from 'lucide-react-native'
import { Pressable, ScrollView, Text, View } from 'react-native'
import { formatBytes } from '~/lib/format-utils'
import type { AttachmentFile } from '../hooks/useAttachments'

interface AttachmentRibbonProps {
    isVisible: boolean
    attachments: AttachmentFile[]
    onRemove: (id: string) => void
}

export function AttachmentRibbon({ isVisible, attachments, onRemove }: AttachmentRibbonProps) {
    const borderColor = useThemeColor('border')

    if (!isVisible) return null

    return (
        <View
            style={{
                borderTopWidth: 1,
                borderTopColor: borderColor,
                paddingVertical: 6,
                paddingHorizontal: 12,
            }}
        >
            <ScrollView horizontal showsHorizontalScrollIndicator={false} style={{ flexGrow: 0 }}>
                <View style={{ flexDirection: 'row', gap: 6 }}>
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
    const [foregroundColor, mutedColor, surfaceColor, borderColor] = useThemeColor([
        'foreground',
        'muted',
        'surface-secondary',
        'border',
    ])

    const truncatedName =
        attachment.name.length > 24 ? `${attachment.name.slice(0, 21)}...` : attachment.name

    return (
        <View
            style={{
                flexDirection: 'row',
                alignItems: 'center',
                gap: 4,
                paddingHorizontal: 8,
                paddingVertical: 4,
                borderRadius: 6,
                borderWidth: 1,
                backgroundColor: surfaceColor,
                borderColor,
            }}
        >
            <Text
                style={{
                    fontSize: 12,
                    fontWeight: '500',
                    maxWidth: 160,
                    color: foregroundColor,
                }}
                numberOfLines={1}
            >
                {truncatedName}
            </Text>
            <Text style={{ fontSize: 11, color: mutedColor }}>{formatBytes(attachment.size)}</Text>
            <Pressable onPress={onRemove} style={{ padding: 2 }} hitSlop={4}>
                <X size={12} color={mutedColor} />
            </Pressable>
        </View>
    )
}
