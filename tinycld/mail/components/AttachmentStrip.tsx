import { formatRelativeDate } from '@tinycld/core/lib/format-utils'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { ChevronDown, ChevronUp, Paperclip } from 'lucide-react-native'
import { useEffect, useRef } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'
import { useAttachmentStripStore } from '../stores/attachment-strip-store'
import { AttachmentThumbnail } from './AttachmentThumbnail'

export interface AttachmentGroup {
    messageId: string
    senderName: string
    date: string
    filenames: string[]
}

interface AttachmentStripProps {
    collectionId: string
    groups: AttachmentGroup[]
    totalCount: number
    isAtBottom: boolean
}

const PANEL_MAX_HEIGHT = 280

export function AttachmentStrip({ collectionId, groups, totalCount, isAtBottom }: AttachmentStripProps) {
    const expanded = useAttachmentStripStore((s) => s.expanded)
    const expand = useAttachmentStripStore((s) => s.expand)
    const toggle = useAttachmentStripStore((s) => s.toggle)

    const wasAtBottomRef = useRef(false)
    useEffect(() => {
        const wasAtBottom = wasAtBottomRef.current
        wasAtBottomRef.current = isAtBottom
        if (!wasAtBottom && isAtBottom && !expanded) {
            expand()
        }
    }, [isAtBottom, expanded, expand])

    if (totalCount === 0) return null

    return (
        <View className="border-t border-border">
            <Header totalCount={totalCount} expanded={expanded} onPress={toggle} />
            <ExpandedPanel
                isVisible={expanded}
                collectionId={collectionId}
                groups={groups}
            />
        </View>
    )
}

function Header({
    totalCount,
    expanded,
    onPress,
}: {
    totalCount: number
    expanded: boolean
    onPress: () => void
}) {
    const mutedColor = useThemeColor('muted-foreground')
    const Chevron = expanded ? ChevronDown : ChevronUp
    const label = `${totalCount} attachment${totalCount === 1 ? '' : 's'}`

    return (
        <Pressable onPress={onPress}>
            <View className="flex-row items-center justify-between px-4 py-2.5">
                <View className="flex-row items-center gap-2">
                    <Paperclip size={16} color={mutedColor} />
                    <Text className="text-sm text-foreground">{label}</Text>
                </View>
                <Chevron size={18} color={mutedColor} />
            </View>
        </Pressable>
    )
}

function ExpandedPanel({
    isVisible,
    collectionId,
    groups,
}: {
    isVisible: boolean
    collectionId: string
    groups: AttachmentGroup[]
}) {
    if (!isVisible) return null

    const showSectionHeaders = groups.length > 1

    return (
        <ScrollView style={{ maxHeight: PANEL_MAX_HEIGHT }} className="border-t border-border">
            <View className="px-4 py-3 gap-3">
                {groups.map((group) => (
                    <GroupBlock
                        key={group.messageId}
                        collectionId={collectionId}
                        group={group}
                        showHeader={showSectionHeaders}
                    />
                ))}
            </View>
        </ScrollView>
    )
}

function GroupBlock({
    collectionId,
    group,
    showHeader,
}: {
    collectionId: string
    group: AttachmentGroup
    showHeader: boolean
}) {
    return (
        <View className="gap-2">
            {showHeader ? (
                <Text
                    className="text-xs text-muted-foreground uppercase"
                    style={{ letterSpacing: 0.5, fontWeight: '600' }}
                >
                    {group.senderName} • {formatRelativeDate(group.date)}
                </Text>
            ) : null}
            <View className="flex-row flex-wrap gap-2">
                {group.filenames.map((filename) => (
                    <AttachmentThumbnail
                        key={filename}
                        collectionId={collectionId}
                        recordId={group.messageId}
                        filename={filename}
                    />
                ))}
            </View>
        </View>
    )
}
