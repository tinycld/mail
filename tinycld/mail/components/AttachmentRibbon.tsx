import { formatBytes } from '@tinycld/core/lib/format-utils'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { X } from 'lucide-react-native'
import { Pressable, ScrollView, Text, View } from 'react-native'
import type { RibbonItem } from '../hooks/useComposeAttachments'

interface AttachmentRibbonProps {
    isVisible: boolean
    items: RibbonItem[]
    onRemove: (item: RibbonItem) => void
    /**
     * [0,1] while the attachments are being uploaded, null otherwise. Sending
     * a few MB used to leave the compose window silent until the request
     * either finished or failed.
     */
    uploadProgress?: number | null
}

export function AttachmentRibbon({
    isVisible,
    items,
    onRemove,
    uploadProgress = null,
}: AttachmentRibbonProps) {
    const borderColor = useThemeColor('border')

    if (!isVisible) return null

    return (
        <View
            className="px-3"
            style={{
                borderTopWidth: 1,
                borderTopColor: borderColor,
                paddingVertical: 6,
            }}
        >
            <ScrollView horizontal showsHorizontalScrollIndicator={false} style={{ flexGrow: 0 }}>
                <View className="flex-row" style={{ gap: 6 }}>
                    {items.map(item => (
                        <AttachmentChip key={item.id} item={item} onRemove={onRemove} />
                    ))}
                </View>
            </ScrollView>
            {uploadProgress !== null ? (
                <View
                    className="h-1 rounded-sm bg-foreground/[0.06] mt-1.5 overflow-hidden"
                    accessibilityRole="progressbar"
                    accessibilityLabel="Uploading attachments"
                    accessibilityValue={{
                        now: Math.round(uploadProgress * 100),
                        min: 0,
                        max: 100,
                    }}
                    testID="mail-attachment-upload-progress"
                >
                    <View
                        className="h-full rounded-sm bg-primary"
                        style={{ width: `${uploadProgress * 100}%` }}
                    />
                </View>
            ) : null}
        </View>
    )
}

function AttachmentChip({
    item,
    onRemove,
}: {
    item: RibbonItem
    onRemove: (item: RibbonItem) => void
}) {
    const foregroundColor = useThemeColor('foreground')
    const mutedColor = useThemeColor('muted-foreground')
    const surfaceColor = useThemeColor('surface-secondary')
    const borderColor = useThemeColor('border')

    const truncatedName = item.name.length > 24 ? `${item.name.slice(0, 21)}...` : item.name

    return (
        <View
            className="flex-row items-center gap-1 px-2 py-1 rounded-md border"
            style={{
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
            <ChipSize size={item.size} />
            <Pressable
                onPress={() => onRemove(item)}
                className="p-0.5"
                hitSlop={4}
                accessibilityRole="button"
                accessibilityLabel={`Remove ${item.name}`}
            >
                <X size={12} color={mutedColor} />
            </Pressable>
        </View>
    )
}

// A draft's saved file has no size on the row, so it shows none rather than 0.
function ChipSize({ size }: { size: number | null }) {
    const mutedColor = useThemeColor('muted-foreground')
    if (size === null) return null
    return <Text style={{ fontSize: 11, color: mutedColor }}>{formatBytes(size)}</Text>
}
