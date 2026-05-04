import { PreviewModal } from '@tinycld/core/file-viewer/PreviewModal'
import type { FilePreviewSource } from '@tinycld/core/file-viewer/types'
import { formatRelativeDate } from '@tinycld/core/lib/format-utils'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { ChevronDown, ChevronUp, Paperclip } from 'lucide-react-native'
import { useEffect, useMemo, useRef } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'
import { useAttachmentPreviewStore } from '../stores/attachment-preview-store'
import { useAttachmentStripStore } from '../stores/attachment-strip-store'
import { AttachmentThumbnail } from './AttachmentThumbnail'
import { attachmentToSource } from './attachment-preview-source'

export interface AttachmentGroup {
    messageId: string
    senderName: string
    date: string
    filenames: string[]
    /** Map of attachment filename → generated thumbnail filename, when available. */
    thumbnailMap?: Record<string, string>
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
    const active = useAttachmentPreviewStore((s) => s.active)
    const closePreview = useAttachmentPreviewStore((s) => s.close)
    const setActive = useAttachmentPreviewStore((s) => s.setActive)
    const openPreview = useAttachmentPreviewStore((s) => s.open)

    const wasAtBottomRef = useRef(false)
    useEffect(() => {
        const wasAtBottom = wasAtBottomRef.current
        wasAtBottomRef.current = isAtBottom
        if (!wasAtBottom && isAtBottom && !expanded) {
            expand()
        }
    }, [isAtBottom, expanded, expand])

    // Flatten every attachment in the visible thread into a single ordered list
    // so the preview modal's prev/next can step across messages, not just within
    // a single message. Recomputed on every render — the store only holds the
    // active (messageId, fileName) identity, so prev/next stay correct even when
    // a message is collapsed/expanded between clicks.
    const flatSources = useMemo(() => {
        const list: FilePreviewSource[] = []
        for (const group of groups) {
            for (const filename of group.filenames) {
                list.push(
                    attachmentToSource({
                        collectionId,
                        recordId: group.messageId,
                        filename,
                        thumbnailFilename: group.thumbnailMap?.[filename],
                    })
                )
            }
        }
        return list
    }, [groups, collectionId])

    if (totalCount === 0) return null

    const activeIndex = active
        ? flatSources.findIndex((s) => s.recordId === active.messageId && s.fileName === active.fileName)
        : -1
    const activeSource = activeIndex >= 0 ? flatSources[activeIndex] : null
    const hasPrevious = activeIndex > 0
    const hasNext = activeIndex >= 0 && activeIndex < flatSources.length - 1

    const handleOpen = (filename: string, recordId: string) => {
        openPreview(recordId, filename)
    }
    const handleNext = () => {
        if (!hasNext) return
        const target = flatSources[activeIndex + 1]
        setActive({ messageId: target.recordId, fileName: target.fileName })
    }
    const handlePrevious = () => {
        if (!hasPrevious) return
        const target = flatSources[activeIndex - 1]
        setActive({ messageId: target.recordId, fileName: target.fileName })
    }

    return (
        <View className="border-t border-border">
            <Header totalCount={totalCount} expanded={expanded} onPress={toggle} />
            <ExpandedPanel
                isVisible={expanded}
                collectionId={collectionId}
                groups={groups}
                onOpen={handleOpen}
            />
            <PreviewModal
                isVisible={activeSource !== null}
                source={activeSource}
                onClose={closePreview}
                onNext={hasNext ? handleNext : undefined}
                onPrevious={hasPrevious ? handlePrevious : undefined}
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
    onOpen,
}: {
    isVisible: boolean
    collectionId: string
    groups: AttachmentGroup[]
    onOpen: (filename: string, recordId: string) => void
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
                        onOpen={onOpen}
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
    onOpen,
}: {
    collectionId: string
    group: AttachmentGroup
    showHeader: boolean
    onOpen: (filename: string, recordId: string) => void
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
                        onPress={() => onOpen(filename, group.messageId)}
                    />
                ))}
            </View>
        </View>
    )
}
