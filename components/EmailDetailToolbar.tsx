import {
    Archive,
    ArrowLeft,
    ChevronLeft,
    ChevronRight,
    CircleAlert,
    FolderInput,
    Forward,
    ListFilter,
    Mail,
    MailOpen,
    MoreVertical,
    Star,
    Tag,
    Trash2,
} from 'lucide-react-native'
import { useRouter } from 'one'
import { StyleSheet, View } from 'react-native'
import { ToolbarSeparator } from '~/components/ToolbarSeparator'
import { useBreakpoint } from '~/components/workspace/useBreakpoint'
import type { MailThreadState } from '../types'
import { MenuActionItem, ToolbarMenu } from './DropdownMenu'
import { ToolbarIconButton } from './ToolbarIconButton'

interface LabelInfo {
    id: string
    name: string
    color: string
}

interface EmailDetailToolbarProps {
    threadState: MailThreadState | undefined
    labels: LabelInfo[]
    threadLabelIds: Set<string>
    onArchive: () => void
    onSpam: () => void
    onTrash: () => void
    onMove: (folder: MailThreadState['folder']) => void
    onUpdateLabel: (labelId: string, add: boolean) => void
    onToggleRead: () => void
    onToggleStar: () => void
    onForwardAll: () => void
    onNewer?: () => void
    onOlder?: () => void
    hasNewer?: boolean
    hasOlder?: boolean
}

const MOVE_FOLDERS: { label: string; folder: MailThreadState['folder'] }[] = [
    { label: 'Inbox', folder: 'inbox' },
    { label: 'Sent', folder: 'sent' },
    { label: 'Drafts', folder: 'drafts' },
    { label: 'Spam', folder: 'spam' },
    { label: 'Trash', folder: 'trash' },
    { label: 'Archive', folder: 'archive' },
]

export function EmailDetailToolbar({
    threadState,
    labels,
    threadLabelIds,
    onArchive,
    onSpam,
    onTrash,
    onMove,
    onUpdateLabel,
    onToggleRead,
    onToggleStar,
    onForwardAll,
    onNewer,
    onOlder,
    hasNewer = false,
    hasOlder = false,
}: EmailDetailToolbarProps) {
    const router = useRouter()
    const breakpoint = useBreakpoint()
    const isMobile = breakpoint === 'mobile'

    const isRead = threadState?.is_read ?? false
    const isStarred = threadState?.is_starred ?? false

    const ReadIcon = isRead ? MailOpen : Mail
    const readLabel = isRead ? 'Mark as unread' : 'Mark as read'

    return (
        <View style={styles.toolbar}>
            <View style={styles.left}>
                <ToolbarIconButton icon={ArrowLeft} label="Back" onPress={() => router.back()} />
                <ToolbarIconButton icon={Archive} label="Archive" onPress={onArchive} />
                {isMobile ? null : (
                    <ToolbarIconButton icon={CircleAlert} label="Report spam" onPress={onSpam} />
                )}
                <ToolbarIconButton icon={Trash2} label="Delete" onPress={onTrash} />
                {isMobile ? null : (
                    <>
                        <ToolbarSeparator />
                        <ToolbarMenu icon={FolderInput} label="Move to">
                            {MOVE_FOLDERS.map(({ label, folder }) => (
                                <MenuActionItem
                                    key={folder}
                                    label={label}
                                    onPress={() => onMove(folder)}
                                />
                            ))}
                        </ToolbarMenu>
                    </>
                )}
                <ToolbarMenu icon={Tag} label="Labels">
                    {labels.map(lbl => {
                        const isActive = threadLabelIds.has(lbl.id)
                        return (
                            <MenuActionItem
                                key={lbl.id}
                                label={lbl.name}
                                colorDot={lbl.color}
                                isActive={isActive}
                                onPress={() => onUpdateLabel(lbl.id, !isActive)}
                            />
                        )
                    })}
                </ToolbarMenu>
                <ToolbarIconButton icon={ReadIcon} label={readLabel} onPress={onToggleRead} />
                <ToolbarMenu icon={MoreVertical} label="More options">
                    <MenuActionItem
                        label={isRead ? 'Mark as unread' : 'Mark as read'}
                        icon={isRead ? MailOpen : Mail}
                        onPress={onToggleRead}
                    />
                    <MenuActionItem
                        label={isStarred ? 'Remove star' : 'Add star'}
                        icon={Star}
                        onPress={onToggleStar}
                    />
                    <MenuActionItem
                        label="Filter messages like these"
                        icon={ListFilter}
                        onPress={() => {}}
                        disabled
                    />
                    <MenuActionItem label="Forward all" icon={Forward} onPress={onForwardAll} />
                </ToolbarMenu>
            </View>
            {isMobile ? null : (
                <View style={styles.right}>
                    <ToolbarIconButton
                        icon={ChevronLeft}
                        label="Newer"
                        onPress={onNewer ?? (() => {})}
                        disabled={!hasNewer}
                    />
                    <ToolbarIconButton
                        icon={ChevronRight}
                        label="Older"
                        onPress={onOlder ?? (() => {})}
                        disabled={!hasOlder}
                    />
                </View>
            )}
        </View>
    )
}

const styles = StyleSheet.create({
    toolbar: {
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'space-between',
        height: 44,
        paddingHorizontal: 8,
        overflow: 'visible',
    },
    left: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 2,
        overflow: 'visible',
    },
    right: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 2,
    },
})
