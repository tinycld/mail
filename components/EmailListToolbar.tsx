import {
    Archive,
    ChevronLeft,
    ChevronRight,
    CircleAlert,
    FolderInput,
    Mail,
    MailOpen,
    MoreVertical,
    RefreshCw,
    Square,
    SquareCheck,
    SquareMinus,
    Star,
    Tag,
    Trash2,
} from 'lucide-react-native'
import { Pressable, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import { useBreakpoint } from '~/components/workspace/useBreakpoint'
import type { MailThreadState } from '../types'
import { MenuActionItem, ToolbarMenu } from './DropdownMenu'
import { ToolbarIconButton } from './ToolbarIconButton'

interface LabelInfo {
    id: string
    name: string
    color: string
}

interface EmailListToolbarProps {
    emailCount: number
    hasSelection: boolean
    selectedCount: number
    allSelected: boolean
    someSelected: boolean
    allSelectedRead: boolean
    allSelectedStarred: boolean
    labels: LabelInfo[]
    selectedItemLabelIds: Set<string>
    onToggleAll: () => void
    onArchive: () => void
    onSpam: () => void
    onTrash: () => void
    onToggleRead: (markAsRead: boolean) => void
    onMove: (folder: MailThreadState['folder']) => void
    onToggleStar: (star: boolean) => void
    onUpdateLabel: (labelId: string, add: boolean) => void
    onRefresh?: () => void
    isRefreshing?: boolean
}

export function EmailListToolbar(props: EmailListToolbarProps) {
    const breakpoint = useBreakpoint()

    if (breakpoint === 'mobile') return null

    if (props.hasSelection) return <BulkActionsToolbar {...props} />
    return <DefaultToolbar {...props} />
}

function DefaultToolbar({
    emailCount,
    onToggleAll,
    onRefresh,
    isRefreshing,
}: EmailListToolbarProps) {
    const theme = useTheme()

    const paginationText =
        emailCount > 0 ? `1\u2013${emailCount} of ${emailCount}` : 'No conversations'

    return (
        <View style={[styles.toolbar, { borderBottomColor: theme.borderColor.val }]}>
            <View style={styles.left}>
                <Pressable style={styles.checkbox} onPress={onToggleAll}>
                    <Square size={18} color={theme.color8.val} />
                </Pressable>
                <ToolbarIconButton
                    icon={RefreshCw}
                    label="Refresh"
                    onPress={onRefresh ?? (() => {})}
                    disabled={isRefreshing}
                />
                <ToolbarIconButton icon={MoreVertical} label="More" onPress={() => {}} />
            </View>
            <View style={styles.right}>
                <Text style={[styles.paginationText, { color: theme.color8.val }]}>
                    {paginationText}
                </Text>
                <ToolbarIconButton icon={ChevronLeft} label="Newer" onPress={() => {}} />
                <ToolbarIconButton icon={ChevronRight} label="Older" onPress={() => {}} />
            </View>
        </View>
    )
}

const MOVE_FOLDERS: { label: string; folder: MailThreadState['folder'] }[] = [
    { label: 'Inbox', folder: 'inbox' },
    { label: 'Sent', folder: 'sent' },
    { label: 'Drafts', folder: 'drafts' },
    { label: 'Spam', folder: 'spam' },
    { label: 'Trash', folder: 'trash' },
    { label: 'Archive', folder: 'archive' },
]

function BulkActionsToolbar({
    emailCount,
    selectedCount,
    allSelected,
    someSelected,
    allSelectedRead,
    allSelectedStarred,
    labels,
    selectedItemLabelIds,
    onToggleAll,
    onArchive,
    onSpam,
    onTrash,
    onToggleRead,
    onMove,
    onToggleStar,
    onUpdateLabel,
}: EmailListToolbarProps) {
    const theme = useTheme()

    const SelectIcon = allSelected ? SquareCheck : someSelected ? SquareMinus : Square

    const paginationText =
        emailCount > 0 ? `1\u2013${emailCount} of ${emailCount}` : 'No conversations'

    const ReadIcon = allSelectedRead ? MailOpen : Mail
    const readLabel = allSelectedRead ? 'Mark as unread' : 'Mark as read'

    return (
        <View style={[styles.toolbar, { borderBottomColor: theme.borderColor.val }]}>
            <View style={styles.left}>
                <Pressable style={styles.checkbox} onPress={onToggleAll}>
                    <SelectIcon size={18} color={theme.accentBackground.val} />
                </Pressable>
                <Text style={[styles.selectedText, { color: theme.color.val }]}>
                    {selectedCount} selected
                </Text>
                <ToolbarIconButton icon={Archive} label="Archive" onPress={onArchive} />
                <ToolbarIconButton icon={CircleAlert} label="Report spam" onPress={onSpam} />
                <ToolbarIconButton icon={Trash2} label="Delete" onPress={onTrash} />
                <Text style={[styles.separator, { color: theme.color8.val }]}>|</Text>
                <ToolbarIconButton
                    icon={ReadIcon}
                    label={readLabel}
                    onPress={() => onToggleRead(!allSelectedRead)}
                />
                <ToolbarMenu icon={FolderInput} label="Move to">
                    {MOVE_FOLDERS.map(({ label, folder }) => (
                        <MenuActionItem key={folder} label={label} onPress={() => onMove(folder)} />
                    ))}
                </ToolbarMenu>
                <ToolbarMenu icon={Tag} label="Labels">
                    {labels.map(lbl => {
                        const isActive = selectedItemLabelIds.has(lbl.id)
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
                <ToolbarMenu icon={MoreVertical} label="More">
                    <MenuActionItem
                        label={allSelectedRead ? 'Mark as unread' : 'Mark as read'}
                        icon={allSelectedRead ? MailOpen : Mail}
                        onPress={() => onToggleRead(!allSelectedRead)}
                    />
                    <MenuActionItem
                        label={allSelectedStarred ? 'Remove star' : 'Add star'}
                        icon={Star}
                        onPress={() => onToggleStar(!allSelectedStarred)}
                    />
                </ToolbarMenu>
            </View>
            <View style={styles.right}>
                <Text style={[styles.paginationText, { color: theme.color8.val }]}>
                    {paginationText}
                </Text>
                <ToolbarIconButton icon={ChevronLeft} label="Newer" onPress={() => {}} />
                <ToolbarIconButton icon={ChevronRight} label="Older" onPress={() => {}} />
            </View>
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
        borderBottomWidth: 1,
        overflow: 'visible',
        zIndex: 1,
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
    checkbox: {
        padding: 8,
    },
    selectedText: {
        fontSize: 13,
        marginHorizontal: 4,
    },
    separator: {
        fontSize: 18,
        marginHorizontal: 4,
    },
    paginationText: {
        fontSize: 12,
        marginRight: 4,
    },
})
