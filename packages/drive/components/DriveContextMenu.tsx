import { ContextMenu } from '@tamagui/context-menu'
import type { LucideIcon } from 'lucide-react-native'
import {
    Download,
    Eye,
    FolderInput,
    FolderOpen,
    Pencil,
    RotateCcw,
    Star,
    StarOff,
    Trash2,
    UserPlus,
} from 'lucide-react-native'
import type { ReactNode } from 'react'
import { useTheme } from 'tamagui'
import { useDrive } from '../hooks/useDrive'
import type { DriveItemView } from '../types'

interface DriveContextMenuProps {
    item: DriveItemView
    children: ReactNode
}

export function DriveContextMenu({ item, children }: DriveContextMenuProps) {
    const theme = useTheme()
    const {
        activeSection,
        openPreview,
        openItem,
        downloadItem,
        toggleStar,
        moveToTrash,
        restoreFromTrash,
        permanentlyDelete,
        canRestoreToOriginalLocation,
        requestRename,
        requestMove,
        requestShare,
    } = useDrive()

    const isTrash = activeSection === 'trash'

    return (
        <ContextMenu>
            <ContextMenu.Trigger>{children}</ContextMenu.Trigger>
            <ContextMenu.Portal zIndex={100}>
                <ContextMenu.Content
                    borderRadius={8}
                    minWidth={200}
                    backgroundColor="$background"
                    borderColor="$borderColor"
                    borderWidth={1}
                    paddingVertical="$1"
                    shadowColor="#000"
                    shadowOffset={{ width: 0, height: 4 }}
                    shadowOpacity={0.15}
                    shadowRadius={12}
                >
                    {isTrash ? (
                        <TrashMenuItems
                            theme={theme}
                            onRestore={() => restoreFromTrash(item.id)}
                            canRestoreToOriginal={canRestoreToOriginalLocation(item.id)}
                            onRequestMove={() => requestMove(item.id, item.name)}
                            onPermanentDelete={() => permanentlyDelete(item.id)}
                        />
                    ) : (
                        <NormalMenuItems
                            item={item}
                            theme={theme}
                            onPreview={() => openPreview(item)}
                            onOpen={() => openItem(item)}
                            onDownload={() => downloadItem(item.id)}
                            onToggleStar={() => toggleStar(item.id)}
                            onShare={() => requestShare(item.id, item.name)}
                            onRename={() => requestRename(item.id, item.name)}
                            onMove={() => requestMove(item.id, item.name)}
                            onTrash={() => moveToTrash(item.id)}
                        />
                    )}
                </ContextMenu.Content>
            </ContextMenu.Portal>
        </ContextMenu>
    )
}

function NormalMenuItems({
    item,
    theme,
    onPreview,
    onOpen,
    onDownload,
    onToggleStar,
    onShare,
    onRename,
    onMove,
    onTrash,
}: {
    item: DriveItemView
    theme: ReturnType<typeof useTheme>
    onPreview: () => void
    onOpen: () => void
    onDownload: () => void
    onToggleStar: () => void
    onShare: () => void
    onRename: () => void
    onMove: () => void
    onTrash: () => void
}) {
    return (
        <>
            {!item.isFolder && (
                <ContextMenuItem label="Preview" icon={Eye} onSelect={onPreview} theme={theme} />
            )}
            <ContextMenuItem label="Open" icon={FolderOpen} onSelect={onOpen} theme={theme} />
            {!item.isFolder && (
                <ContextMenuItem
                    label="Download"
                    icon={Download}
                    onSelect={onDownload}
                    theme={theme}
                />
            )}
            <ContextMenu.Separator borderColor="$borderColor" marginVertical="$1" />
            <ContextMenuItem
                label={item.starred ? 'Remove star' : 'Add star'}
                icon={item.starred ? StarOff : Star}
                onSelect={onToggleStar}
                theme={theme}
            />
            <ContextMenuItem label="Share" icon={UserPlus} onSelect={onShare} theme={theme} />
            <ContextMenuItem label="Rename" icon={Pencil} onSelect={onRename} theme={theme} />
            <ContextMenuItem label="Move" icon={FolderInput} onSelect={onMove} theme={theme} />
            <ContextMenu.Separator borderColor="$borderColor" marginVertical="$1" />
            <ContextMenuItem label="Move to trash" icon={Trash2} onSelect={onTrash} theme={theme} />
        </>
    )
}

function TrashMenuItems({
    theme,
    onRestore,
    canRestoreToOriginal,
    onRequestMove,
    onPermanentDelete,
}: {
    theme: ReturnType<typeof useTheme>
    onRestore: () => void
    canRestoreToOriginal: boolean
    onRequestMove: () => void
    onPermanentDelete: () => void
}) {
    const handleRestore = canRestoreToOriginal ? onRestore : onRequestMove

    return (
        <>
            <ContextMenuItem
                label={canRestoreToOriginal ? 'Restore' : 'Restore to...'}
                icon={RotateCcw}
                onSelect={handleRestore}
                theme={theme}
            />
            <ContextMenu.Separator borderColor="$borderColor" marginVertical="$1" />
            <ContextMenuItem
                label="Delete permanently"
                icon={Trash2}
                onSelect={onPermanentDelete}
                theme={theme}
            />
        </>
    )
}

function ContextMenuItem({
    label,
    icon: Icon,
    onSelect,
    theme,
}: {
    label: string
    icon: LucideIcon
    onSelect: () => void
    theme: ReturnType<typeof useTheme>
}) {
    return (
        <ContextMenu.Item onSelect={onSelect} gap="$2">
            <ContextMenu.ItemIcon>
                <Icon size={16} color={theme.color8.val} />
            </ContextMenu.ItemIcon>
            <ContextMenu.ItemTitle size="$3">{label}</ContextMenu.ItemTitle>
        </ContextMenu.Item>
    )
}
