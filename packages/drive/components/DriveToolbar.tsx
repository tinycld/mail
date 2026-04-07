import { Menu } from '@tamagui/menu'
import {
    ChevronDown,
    ChevronRight,
    Download,
    Eye,
    FolderInput,
    FolderPlus,
    Grid,
    Info,
    List,
    MoreVertical,
    Pencil,
    RotateCcw,
    Search,
    Trash2,
    Upload,
    UserPlus,
    X,
} from 'lucide-react-native'
import { useCallback, useEffect, useState } from 'react'
import { Platform, Pressable, StyleSheet, Text, View } from 'react-native'
import { Button, Dialog, useTheme, XStack } from 'tamagui'
import { MenuActionItem } from '~/components/DropdownMenu'
import { ConfirmTrash, SuretyGuard } from '~/components/SuretyGuard'
import { ToolbarIconButton } from '~/components/ToolbarIconButton'
import { captureException } from '~/lib/errors'
import { useCurrentRole } from '~/lib/use-current-role'
import { useOrgSlug } from '~/lib/use-org-slug'
import { PlainInput } from '~/ui/PlainInput'
import { useDrive } from '../hooks/useDrive'
import type { DriveItemView, ViewMode } from '../types'
import { ChooseFolderDialog } from './ChooseFolderDialog'
import { ShareDialog } from './ShareDialog'

type PromptState =
    | { type: 'closed' }
    | { type: 'new-folder' }
    | { type: 'rename'; itemId: string; currentName: string }

export function DriveToolbar() {
    const theme = useTheme()
    const {
        selectedItem,
        activeSection,
        breadcrumbs,
        currentFolderId,
        viewMode,
        setViewMode,
        selectItem,
        navigateToFolder,
        searchQuery,
        setSearchQuery,
        isSearching,
        triggerFilePicker,
        createFolder,
        renameItem,
        downloadItem,
        moveItem,
        moveToTrash,
        folderTree,
        removeShare,
        getSharesForItem,
        orgMembers,
        pendingRename,
        pendingMove,
        pendingShare,
        clearPendingRename,
        clearPendingMove,
        clearPendingShare,
    } = useDrive()
    const { userOrgId } = useCurrentRole()
    const orgSlug = useOrgSlug()

    const [prompt, setPrompt] = useState<PromptState>({ type: 'closed' })
    const [promptKey, setPromptKey] = useState(0)
    const [moveTarget, setMoveTarget] = useState<{ id: string; name: string } | null>(null)
    const [shareTarget, setShareTarget] = useState<{ id: string; name: string } | null>(null)

    const handlePromptSubmit = useCallback(
        (value: string) => {
            if (prompt.type === 'new-folder') {
                createFolder(value)
            } else if (prompt.type === 'rename') {
                renameItem(prompt.itemId, value)
            }
            setPrompt({ type: 'closed' })
        },
        [prompt, createFolder, renameItem]
    )

    const openPrompt = useCallback((state: PromptState) => {
        setPrompt(state)
        setPromptKey(k => k + 1)
    }, [])

    useEffect(() => {
        if (pendingRename) {
            openPrompt({
                type: 'rename',
                itemId: pendingRename.id,
                currentName: pendingRename.name,
            })
            clearPendingRename()
        }
    }, [pendingRename, openPrompt, clearPendingRename])

    useEffect(() => {
        if (pendingMove) {
            setMoveTarget({ id: pendingMove.id, name: pendingMove.name })
            clearPendingMove()
        }
    }, [pendingMove, clearPendingMove])

    useEffect(() => {
        if (pendingShare) {
            setShareTarget({ id: pendingShare.id, name: pendingShare.name })
            clearPendingShare()
        }
    }, [pendingShare, clearPendingShare])

    const promptDialog = (
        <NamePromptDialog
            key={promptKey}
            open={prompt.type !== 'closed'}
            title={prompt.type === 'new-folder' ? 'New folder' : 'Rename'}
            placeholder={prompt.type === 'new-folder' ? 'Untitled folder' : ''}
            defaultValue={prompt.type === 'rename' ? prompt.currentName : ''}
            submitLabel={prompt.type === 'new-folder' ? 'Create' : 'Rename'}
            onSubmit={handlePromptSubmit}
            onClose={() => setPrompt({ type: 'closed' })}
        />
    )

    const moveDialog = (
        <ChooseFolderDialog
            open={moveTarget !== null}
            itemName={moveTarget?.name ?? ''}
            excludeId={moveTarget?.id ?? ''}
            folderTree={folderTree}
            onMove={targetId => {
                if (moveTarget) {
                    moveItem(moveTarget.id, targetId)
                    selectItem(null)
                }
            }}
            onClose={() => setMoveTarget(null)}
        />
    )

    const shareDialog = (
        <ShareDialog
            open={shareTarget !== null}
            itemId={shareTarget?.id ?? ''}
            itemName={shareTarget?.name ?? ''}
            orgSlug={orgSlug}
            shares={shareTarget ? getSharesForItem(shareTarget.id) : []}
            orgMembers={orgMembers}
            currentUserOrgId={userOrgId}
            onRemoveShare={removeShare}
            onClose={() => setShareTarget(null)}
        />
    )

    if (selectedItem) {
        return (
            <>
                <SelectionToolbar
                    item={selectedItem}
                    viewMode={viewMode}
                    onSetViewMode={setViewMode}
                    onClearSelection={() => selectItem(null)}
                    onOpenRename={(itemId, name) =>
                        openPrompt({ type: 'rename', itemId, currentName: name })
                    }
                    onOpenMove={(itemId, name) => setMoveTarget({ id: itemId, name })}
                    onOpenShare={(itemId, name) => setShareTarget({ id: itemId, name })}
                    theme={theme}
                />
                {promptDialog}
                {moveDialog}
                {shareDialog}
            </>
        )
    }

    const isSearchActive = searchQuery.length >= 2

    const leftContent = (() => {
        if (isSearchActive) {
            return (
                <Text style={[styles.searchLabel, { color: theme.color8.val }]}>
                    Search results{isSearching ? '...' : ''}
                </Text>
            )
        }
        if (activeSection === 'trash') {
            return <Text style={[styles.trashTitle, { color: theme.color.val }]}>Trash</Text>
        }
        return (
            <Breadcrumbs
                breadcrumbs={breadcrumbs}
                currentFolderId={currentFolderId}
                onNavigate={navigateToFolder}
                onUpload={triggerFilePicker}
                onNewFolder={() => openPrompt({ type: 'new-folder' })}
                onRename={
                    currentFolderId
                        ? () => {
                              const current = breadcrumbs.at(-1)
                              if (current)
                                  openPrompt({
                                      type: 'rename',
                                      itemId: current.id,
                                      currentName: current.name,
                                  })
                          }
                        : undefined
                }
                onDownload={downloadItem}
                onTrash={moveToTrash}
            />
        )
    })()

    return (
        <>
            <View style={[styles.toolbar, { borderBottomColor: theme.borderColor.val }]}>
                {leftContent}
                <View style={styles.rightSection}>
                    <SearchInput value={searchQuery} onChangeText={setSearchQuery} theme={theme} />
                    <ViewToggle viewMode={viewMode} onSetViewMode={setViewMode} theme={theme} />
                </View>
            </View>
            {promptDialog}
            {moveDialog}
            {shareDialog}
        </>
    )
}

function Breadcrumbs({
    breadcrumbs,
    currentFolderId,
    onNavigate,
    onUpload,
    onNewFolder,
    onRename,
    onDownload,
    onTrash,
}: {
    breadcrumbs: DriveItemView[]
    currentFolderId: string
    onNavigate: (folderId: string) => void
    onUpload: () => void
    onNewFolder: () => void
    onRename?: () => void
    onDownload: (itemId: string) => void
    onTrash: (itemId: string) => void
}) {
    const theme = useTheme()
    const ancestors = breadcrumbs.slice(0, -1)
    const current = breadcrumbs.at(-1)
    const currentLabel = current?.name ?? 'My Files'
    const isInsideFolder = currentFolderId !== ''

    return (
        <View style={styles.breadcrumbs}>
            {ancestors.length > 0 && (
                <>
                    <Pressable onPress={() => onNavigate('')} style={styles.breadcrumbButton}>
                        <Text
                            style={[styles.ancestorText, { color: theme.color.val }]}
                            numberOfLines={1}
                        >
                            My Files
                        </Text>
                    </Pressable>
                    <ChevronRight size={16} color={theme.color8.val} />
                </>
            )}
            {ancestors.map(crumb => (
                <View key={crumb.id} style={styles.breadcrumbSegment}>
                    <Pressable onPress={() => onNavigate(crumb.id)} style={styles.breadcrumbButton}>
                        <Text
                            style={[styles.ancestorText, { color: theme.color.val }]}
                            numberOfLines={1}
                        >
                            {crumb.name}
                        </Text>
                    </Pressable>
                    <ChevronRight size={16} color={theme.color8.val} />
                </View>
            ))}

            <Menu>
                <Menu.Trigger asChild>
                    <Pressable
                        style={[styles.currentCrumb, { borderColor: theme.borderColor.val }]}
                    >
                        <Text
                            style={[styles.currentCrumbText, { color: theme.color.val }]}
                            numberOfLines={1}
                        >
                            {currentLabel}
                        </Text>
                        <ChevronDown size={16} color={theme.color8.val} />
                    </Pressable>
                </Menu.Trigger>
                <Menu.Portal zIndex={100}>
                    <Menu.Content
                        borderRadius={8}
                        minWidth={220}
                        backgroundColor="$background"
                        borderColor="$borderColor"
                        borderWidth={1}
                        paddingVertical="$1"
                        shadowColor="#000"
                        shadowOffset={{ width: 0, height: 4 }}
                        shadowOpacity={0.15}
                        shadowRadius={12}
                    >
                        <MenuActionItem
                            label="New folder"
                            icon={FolderPlus}
                            onPress={onNewFolder}
                        />
                        <MenuActionItem label="Upload file" icon={Upload} onPress={onUpload} />
                        {isInsideFolder && (
                            <MenuActionItem
                                label="Download"
                                icon={Download}
                                onPress={() => onDownload(currentFolderId)}
                            />
                        )}
                        {isInsideFolder && onRename && (
                            <MenuActionItem label="Rename" icon={Pencil} onPress={onRename} />
                        )}
                        <MenuActionItem label="Share" icon={UserPlus} onPress={() => {}} />
                        <MenuActionItem label="Folder information" icon={Info} onPress={() => {}} />
                    </Menu.Content>
                </Menu.Portal>
            </Menu>
            {isInsideFolder && (
                <ConfirmTrash
                    itemName={currentLabel}
                    onConfirmed={() => {
                        onTrash(currentFolderId)
                        onNavigate('')
                    }}
                >
                    {onOpen => (
                        <Pressable style={styles.breadcrumbTrash} onPress={onOpen}>
                            <Trash2 size={14} color={theme.color8.val} />
                        </Pressable>
                    )}
                </ConfirmTrash>
            )}
        </View>
    )
}

interface SearchInputProps {
    value: string
    onChangeText: (text: string) => void
    theme: ReturnType<typeof useTheme>
}

function SearchInput({ value, onChangeText, theme }: SearchInputProps) {
    return (
        <View style={[styles.searchContainer, { borderColor: theme.borderColor.val }]}>
            <Search size={14} color={theme.color8.val} />
            <PlainInput
                style={[styles.searchInput, { color: theme.color.val }]}
                placeholder="Search in Files"
                placeholderTextColor={theme.color8.val}
                value={value}
                onChangeText={onChangeText}
            />
            {value.length > 0 && (
                <Pressable onPress={() => onChangeText('')} hitSlop={8}>
                    <X size={14} color={theme.color8.val} />
                </Pressable>
            )}
        </View>
    )
}

interface SelectionToolbarProps {
    item: { name: string; isFolder: boolean; id: string }
    viewMode: ViewMode
    onSetViewMode: (mode: ViewMode) => void
    onClearSelection: () => void
    onOpenRename: (itemId: string, currentName: string) => void
    onOpenMove: (itemId: string, name: string) => void
    onOpenShare: (itemId: string, name: string) => void
    theme: ReturnType<typeof useTheme>
}

function SelectionToolbar({
    item,
    viewMode,
    onSetViewMode,
    onClearSelection,
    onOpenRename,
    onOpenMove,
    onOpenShare,
    theme,
}: SelectionToolbarProps) {
    const {
        activeSection,
        uploadNewVersion,
        downloadItem,
        moveToTrash,
        restoreFromTrash,
        permanentlyDelete,
        canRestoreToOriginalLocation,
        restoreToFolder,
        folderTree,
        openPreview,
        selectedItem,
    } = useDrive()
    const [restoreMoveTarget, setRestoreMoveTarget] = useState<string | null>(null)

    const isTrash = activeSection === 'trash'

    const triggerVersionUpload = () => {
        if (Platform.OS === 'web') {
            const input = document.createElement('input')
            input.type = 'file'
            input.onchange = () => {
                if (input.files?.[0]) {
                    uploadNewVersion(item.id, input.files[0]).catch(err =>
                        captureException('uploadNewVersion', err)
                    )
                }
            }
            input.click()
        }
    }

    if (isTrash) {
        const handleRestore = () => {
            if (canRestoreToOriginalLocation(item.id)) {
                restoreFromTrash(item.id)
                onClearSelection()
            } else {
                setRestoreMoveTarget(item.id)
            }
        }

        return (
            <>
                <View style={[styles.toolbar, { borderBottomColor: theme.borderColor.val }]}>
                    <View style={styles.selectionLeft}>
                        <Pressable onPress={onClearSelection} style={styles.closeButton}>
                            <X size={16} color={theme.color8.val} />
                        </Pressable>
                        <Text
                            style={[styles.selectionText, { color: theme.color.val }]}
                            numberOfLines={1}
                        >
                            {item.name}
                        </Text>
                    </View>
                    <View style={styles.actions}>
                        <ToolbarIconButton
                            icon={RotateCcw}
                            label="Restore"
                            onPress={handleRestore}
                        />
                        <SuretyGuard
                            message={`Permanently delete "${item.name}"? This cannot be undone.`}
                            confirmLabel="Delete permanently"
                            onConfirmed={() => {
                                permanentlyDelete(item.id)
                                onClearSelection()
                            }}
                        >
                            {onOpen => (
                                <ToolbarIconButton
                                    icon={Trash2}
                                    label="Delete permanently"
                                    onPress={onOpen}
                                />
                            )}
                        </SuretyGuard>
                        <View
                            style={[
                                styles.actionDivider,
                                { backgroundColor: theme.borderColor.val },
                            ]}
                        />
                        <ViewToggle
                            viewMode={viewMode}
                            onSetViewMode={onSetViewMode}
                            theme={theme}
                        />
                    </View>
                </View>
                <ChooseFolderDialog
                    open={restoreMoveTarget !== null}
                    itemName={item.name}
                    excludeId={item.id}
                    folderTree={folderTree}
                    title="Original location has been removed, select alternative location"
                    confirmLabel="Restore here"
                    onMove={targetId => {
                        if (restoreMoveTarget) {
                            restoreToFolder(restoreMoveTarget, targetId)
                            onClearSelection()
                        }
                    }}
                    onClose={() => setRestoreMoveTarget(null)}
                />
            </>
        )
    }

    const actionIcons = [
        ...(!item.isFolder
            ? [
                  {
                      key: 'preview',
                      icon: Eye,
                      label: 'Preview',
                      onPress: () => {
                          if (selectedItem) openPreview(selectedItem)
                      },
                  },
                  {
                      key: 'upload-version',
                      icon: Upload,
                      label: 'Upload new version',
                      onPress: triggerVersionUpload,
                  },
              ]
            : []),
        {
            key: 'share',
            icon: UserPlus,
            label: 'Share',
            onPress: () => onOpenShare(item.id, item.name),
        },
        {
            key: 'download',
            icon: Download,
            label: 'Download',
            onPress: () => downloadItem(item.id),
        },
        {
            key: 'rename',
            icon: Pencil,
            label: 'Rename',
            onPress: () => onOpenRename(item.id, item.name),
        },
        {
            key: 'move',
            icon: FolderInput,
            label: 'Move',
            onPress: () => onOpenMove(item.id, item.name),
        },
        { key: 'more', icon: MoreVertical, label: 'More actions', onPress: () => {} },
    ]

    return (
        <View style={[styles.toolbar, { borderBottomColor: theme.borderColor.val }]}>
            <View style={styles.selectionLeft}>
                <Pressable onPress={onClearSelection} style={styles.closeButton}>
                    <X size={16} color={theme.color8.val} />
                </Pressable>
                <Text style={[styles.selectionText, { color: theme.color.val }]} numberOfLines={1}>
                    {item.name}
                </Text>
            </View>
            <View style={styles.actions}>
                {actionIcons.map(({ key, icon, label, onPress }) => (
                    <ToolbarIconButton key={key} icon={icon} label={label} onPress={onPress} />
                ))}
                <ConfirmTrash
                    itemName={item.name}
                    onConfirmed={() => {
                        moveToTrash(item.id)
                        onClearSelection()
                    }}
                >
                    {onOpen => <ToolbarIconButton icon={Trash2} label="Trash" onPress={onOpen} />}
                </ConfirmTrash>
                <View style={[styles.actionDivider, { backgroundColor: theme.borderColor.val }]} />
                <ViewToggle viewMode={viewMode} onSetViewMode={onSetViewMode} theme={theme} />
            </View>
        </View>
    )
}

interface ViewToggleProps {
    viewMode: ViewMode
    onSetViewMode: (mode: ViewMode) => void
    theme: ReturnType<typeof useTheme>
}

function ViewToggle({ viewMode, onSetViewMode, theme }: ViewToggleProps) {
    return (
        <View style={styles.viewToggle}>
            <Pressable
                onPress={() => onSetViewMode('list')}
                style={[
                    styles.viewButton,
                    viewMode === 'list' && {
                        backgroundColor: `${theme.activeIndicator.val}18`,
                    },
                ]}
            >
                <List
                    size={18}
                    color={viewMode === 'list' ? theme.activeIndicator.val : theme.color8.val}
                />
            </Pressable>
            <Pressable
                onPress={() => onSetViewMode('grid')}
                style={[
                    styles.viewButton,
                    viewMode === 'grid' && {
                        backgroundColor: `${theme.activeIndicator.val}18`,
                    },
                ]}
            >
                <Grid
                    size={18}
                    color={viewMode === 'grid' ? theme.activeIndicator.val : theme.color8.val}
                />
            </Pressable>
        </View>
    )
}

interface NamePromptDialogProps {
    open: boolean
    title: string
    placeholder: string
    defaultValue: string
    submitLabel: string
    onSubmit: (value: string) => void
    onClose: () => void
}

function NamePromptDialog({
    open,
    title,
    placeholder,
    defaultValue,
    submitLabel,
    onSubmit,
    onClose,
}: NamePromptDialogProps) {
    const [value, setValue] = useState(defaultValue)

    const handleSubmit = () => {
        const trimmed = value.trim()
        if (trimmed) onSubmit(trimmed)
    }

    return (
        <Dialog
            modal
            open={open}
            onOpenChange={o => {
                if (!o) onClose()
            }}
        >
            <Dialog.Portal>
                <Dialog.Overlay
                    key="overlay"
                    opacity={0.3}
                    backgroundColor="$shadow6"
                    enterStyle={{ opacity: 0 }}
                    exitStyle={{ opacity: 0 }}
                />
                <Dialog.Content
                    key="content"
                    bordered
                    elevate
                    padding="$4"
                    gap="$3"
                    width={360}
                    backgroundColor="$background"
                >
                    <Dialog.Title size="$5">{title}</Dialog.Title>
                    <XStack
                        borderWidth={1}
                        borderColor="$borderColor"
                        borderRadius={8}
                        paddingHorizontal={12}
                        paddingVertical={10}
                    >
                        <PlainInput
                            value={value}
                            onChangeText={setValue}
                            placeholder={placeholder}
                            autoFocus
                            onSubmitEditing={handleSubmit}
                            style={{ flex: 1, fontSize: 15 }}
                        />
                    </XStack>
                    <XStack gap="$3" justifyContent="flex-end">
                        <Dialog.Close asChild>
                            <Button size="$3" chromeless>
                                <Button.Text>Cancel</Button.Text>
                            </Button>
                        </Dialog.Close>
                        <Button
                            size="$3"
                            theme="accent"
                            onPress={handleSubmit}
                            disabled={!value.trim()}
                        >
                            <Button.Text fontWeight="600">{submitLabel}</Button.Text>
                        </Button>
                    </XStack>
                </Dialog.Content>
            </Dialog.Portal>
        </Dialog>
    )
}

const styles = StyleSheet.create({
    toolbar: {
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'space-between',
        paddingHorizontal: 16,
        paddingVertical: 10,
        borderBottomWidth: 1,
    },
    breadcrumbs: {
        flexDirection: 'row',
        alignItems: 'center',
        flex: 1,
        gap: 2,
        minWidth: 0,
        overflow: 'hidden',
    },
    breadcrumbSegment: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 2,
        flexShrink: 1,
        minWidth: 0,
    },
    breadcrumbButton: {
        flexShrink: 1,
        minWidth: 0,
    },
    ancestorText: {
        fontSize: 18,
        fontWeight: '400',
    },
    currentCrumb: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 4,
        paddingHorizontal: 10,
        paddingVertical: 4,
        borderRadius: 20,
        borderWidth: 1,
        flexShrink: 1,
        minWidth: 0,
        maxWidth: 280,
    },
    currentCrumbText: {
        fontSize: 18,
        fontWeight: '500',
        flexShrink: 1,
    },
    rightSection: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 8,
        flexShrink: 0,
    },
    searchContainer: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 6,
        borderWidth: 1,
        borderRadius: 8,
        paddingHorizontal: 10,
        paddingVertical: 6,
        width: 240,
    },
    searchInput: {
        flex: 1,
        fontSize: 13,
        padding: 0,
    },
    searchLabel: {
        fontSize: 14,
        fontWeight: '500',
        flex: 1,
    },
    trashTitle: {
        fontSize: 18,
        fontWeight: '500',
        flex: 1,
    },
    viewToggle: {
        flexDirection: 'row',
        gap: 2,
    },
    viewButton: {
        padding: 6,
        borderRadius: 6,
    },
    selectionLeft: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 8,
        flex: 1,
    },
    closeButton: {
        padding: 4,
    },
    selectionText: {
        fontSize: 14,
        fontWeight: '500',
        flex: 1,
    },
    actions: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 4,
    },
    actionDivider: {
        width: 1,
        height: 20,
        marginHorizontal: 4,
    },
    breadcrumbTrash: {
        padding: 6,
        marginLeft: 4,
    },
})
