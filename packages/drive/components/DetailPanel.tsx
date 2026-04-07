import { Download, FolderOpen, RotateCcw, X } from 'lucide-react-native'
import { useCallback, useState } from 'react'
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native'
import { Button, Dialog, SizableText, useTheme, XStack } from 'tamagui'
import { formatBytes, formatDate } from '~/lib/format-utils'
import { pb } from '~/lib/pocketbase'
import { useDrive } from '../hooks/useDrive'
import { useVersionHistory } from '../hooks/useVersionHistory'
import type { DriveItemView } from '../types'
import { Thumbnail } from './Thumbnail'

interface DetailPanelProps {
    isVisible: boolean
    item: DriveItemView | undefined
    onClose: () => void
}

export function DetailPanel({ isVisible, item, onClose }: DetailPanelProps) {
    if (!isVisible || !item) return null

    return <DetailPanelContent item={item} onClose={onClose} />
}

type DetailTab = 'details' | 'versions' | 'activity'

function DetailPanelContent({ item, onClose }: { item: DriveItemView; onClose: () => void }) {
    const theme = useTheme()
    const [activeTab, setActiveTab] = useState<DetailTab>('details')
    const showVersionsTab = !item.isFolder

    return (
        <View style={[styles.container, { borderLeftColor: theme.borderColor.val }]}>
            <View style={[styles.header, { borderBottomColor: theme.borderColor.val }]}>
                <Text style={[styles.headerTitle, { color: theme.color.val }]} numberOfLines={2}>
                    {item.name}
                </Text>
                <Pressable onPress={onClose} style={styles.closeButton}>
                    <X size={18} color={theme.color8.val} />
                </Pressable>
            </View>

            <View style={styles.iconArea}>
                <Thumbnail item={item} size={120} />
            </View>

            <TabBar
                tabs={
                    showVersionsTab
                        ? (['details', 'versions', 'activity'] as const)
                        : (['details', 'activity'] as const)
                }
                activeTab={activeTab}
                onTabPress={setActiveTab}
                theme={theme}
            />

            {activeTab === 'details' && <DetailsContent item={item} />}
            {activeTab === 'versions' && showVersionsTab && <VersionsContent itemId={item.id} />}
            {activeTab === 'activity' && <ActivityContent />}
        </View>
    )
}

function DetailsContent({ item }: { item: DriveItemView }) {
    const theme = useTheme()
    const { activeSection, getItemPath } = useDrive()
    const accessText = item.shared ? 'Shared with others' : 'Private to you'
    const isTrash = activeSection === 'trash'
    const originalLocation = isTrash ? getItemPath(item.parentId) : null

    return (
        <View style={styles.content}>
            {isTrash && (
                <>
                    <View style={styles.section}>
                        <Text style={[styles.sectionTitle, { color: theme.color.val }]}>
                            Original location
                        </Text>
                        <View style={styles.accessRow}>
                            <FolderOpen size={16} color={theme.color8.val} />
                            <Text style={[styles.accessText, { color: theme.color8.val }]}>
                                {originalLocation}
                            </Text>
                        </View>
                        <DetailRow label="Deleted" value={formatDate(item.trashedAt)} />
                    </View>

                    <View style={[styles.divider, { backgroundColor: theme.borderColor.val }]} />
                </>
            )}

            <View style={styles.section}>
                <Text style={[styles.sectionTitle, { color: theme.color.val }]}>
                    Who has access
                </Text>
                <View style={styles.accessRow}>
                    <View
                        style={[
                            styles.avatarCircle,
                            { backgroundColor: theme.accentBackground.val },
                        ]}
                    >
                        <Text style={[styles.avatarText, { color: theme.accentColor.val }]}>
                            {item.owner === 'me' ? 'Y' : item.owner.charAt(0)}
                        </Text>
                    </View>
                    <Text style={[styles.accessText, { color: theme.color8.val }]}>
                        {accessText}
                    </Text>
                </View>
            </View>

            <View style={[styles.divider, { backgroundColor: theme.borderColor.val }]} />

            <View style={styles.section}>
                <Text style={[styles.sectionTitle, { color: theme.color.val }]}>File details</Text>
                <DetailRow label="Type" value={item.mimeType} />
                <DetailRow label="Size" value={formatBytes(item.size)} />
                <DetailRow label="Owner" value={item.owner} />
                <DetailRow label="Modified" value={formatDate(item.updated)} />
            </View>
        </View>
    )
}

function DetailRow({ label, value }: { label: string; value: string }) {
    const theme = useTheme()
    return (
        <View style={styles.detailRow}>
            <Text style={[styles.detailLabel, { color: theme.color8.val }]}>{label}</Text>
            <Text style={[styles.detailValue, { color: theme.color.val }]} numberOfLines={1}>
                {value}
            </Text>
        </View>
    )
}

interface TabBarProps {
    tabs: readonly DetailTab[]
    activeTab: DetailTab
    onTabPress: (tab: DetailTab) => void
    theme: ReturnType<typeof useTheme>
}

function TabBar({ tabs, activeTab, onTabPress, theme }: TabBarProps) {
    const labels: Record<DetailTab, string> = {
        details: 'Details',
        versions: 'Versions',
        activity: 'Activity',
    }

    return (
        <View style={[styles.tabs, { borderBottomColor: theme.borderColor.val }]}>
            {tabs.map(tab => (
                <Pressable
                    key={tab}
                    style={[
                        styles.tab,
                        activeTab === tab && {
                            borderBottomColor: theme.accentColor.val,
                            borderBottomWidth: 2,
                        },
                    ]}
                    onPress={() => onTabPress(tab)}
                >
                    <Text
                        style={[
                            styles.tabText,
                            {
                                color: activeTab === tab ? theme.accentColor.val : theme.color8.val,
                            },
                        ]}
                    >
                        {labels[tab]}
                    </Text>
                </Pressable>
            ))}
        </View>
    )
}

function VersionsContent({ itemId }: { itemId: string }) {
    const theme = useTheme()
    const { versions, restoreVersion, isRestoring } = useVersionHistory(itemId)
    const [confirmVersionId, setConfirmVersionId] = useState<string | null>(null)

    const confirmingVersion = confirmVersionId
        ? versions.find(v => v.id === confirmVersionId)
        : null

    const handleConfirmRestore = useCallback(() => {
        if (!confirmVersionId) return
        restoreVersion(confirmVersionId)
        setConfirmVersionId(null)
    }, [confirmVersionId, restoreVersion])

    const handleDownload = useCallback((version: { id: string; file: string }) => {
        const url = pb.files.getURL(
            {
                id: version.id,
                collectionId: 'pbc_drive_versions_01',
                collectionName: 'drive_item_versions',
            },
            version.file
        )
        if (typeof window !== 'undefined') {
            window.open(url, '_blank')
        }
    }, [])

    if (versions.length === 0) {
        return (
            <View style={styles.content}>
                <Text style={[styles.placeholderText, { color: theme.color8.val }]}>
                    No previous versions
                </Text>
            </View>
        )
    }

    return (
        <>
            <ScrollView style={styles.content}>
                {versions.map(version => (
                    <VersionRow
                        key={version.id}
                        version={version}
                        onRestore={() => setConfirmVersionId(version.id)}
                        onDownload={() => handleDownload(version)}
                        isRestoring={isRestoring}
                    />
                ))}
            </ScrollView>

            <RestoreConfirmDialog
                open={!!confirmingVersion}
                onOpenChange={open => {
                    if (!open) setConfirmVersionId(null)
                }}
                versionNumber={confirmingVersion?.version_number ?? 0}
                onConfirm={handleConfirmRestore}
            />
        </>
    )
}

interface RestoreConfirmDialogProps {
    open: boolean
    onOpenChange: (open: boolean) => void
    versionNumber: number
    onConfirm: () => void
}

function RestoreConfirmDialog({
    open,
    onOpenChange,
    versionNumber,
    onConfirm,
}: RestoreConfirmDialogProps) {
    return (
        <Dialog modal open={open} onOpenChange={onOpenChange}>
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
                    width={340}
                    backgroundColor="$background"
                >
                    <Dialog.Title size="$5">Restore version</Dialog.Title>
                    <SizableText size="$3" color="$color10">
                        Restore to version {versionNumber}? The current file will be saved as a new
                        version before restoring.
                    </SizableText>
                    <XStack gap="$3" justifyContent="flex-end">
                        <Dialog.Close asChild>
                            <Button size="$3" chromeless>
                                <Button.Text>Cancel</Button.Text>
                            </Button>
                        </Dialog.Close>
                        <Button
                            size="$3"
                            theme="accent"
                            onPress={() => {
                                onConfirm()
                                onOpenChange(false)
                            }}
                        >
                            <Button.Text fontWeight="600">Restore</Button.Text>
                        </Button>
                    </XStack>
                </Dialog.Content>
            </Dialog.Portal>
        </Dialog>
    )
}

interface VersionRowProps {
    version: {
        id: string
        version_number: number
        size: number
        created: string
        file: string
    }
    onRestore: () => void
    onDownload: () => void
    isRestoring: boolean
}

function VersionRow({ version, onRestore, onDownload, isRestoring }: VersionRowProps) {
    const theme = useTheme()

    return (
        <View style={[styles.versionRow, { borderBottomColor: theme.borderColor.val }]}>
            <View style={styles.versionInfo}>
                <Text style={[styles.versionNumber, { color: theme.color.val }]}>
                    Version {version.version_number}
                </Text>
                <Text style={[styles.versionMeta, { color: theme.color8.val }]}>
                    {formatDate(version.created)} · {formatBytes(version.size)}
                </Text>
            </View>
            <View style={styles.versionActions}>
                <Pressable onPress={onDownload} hitSlop={8} style={styles.versionActionButton}>
                    <Download size={14} color={theme.color8.val} />
                </Pressable>
                <Pressable
                    onPress={onRestore}
                    hitSlop={8}
                    disabled={isRestoring}
                    style={styles.versionActionButton}
                >
                    {isRestoring ? (
                        <ActivityIndicator size="small" />
                    ) : (
                        <RotateCcw size={14} color={theme.color8.val} />
                    )}
                </Pressable>
            </View>
        </View>
    )
}

function ActivityContent() {
    const theme = useTheme()
    return (
        <View style={styles.content}>
            <Text style={[styles.placeholderText, { color: theme.color8.val }]}>
                No recent activity
            </Text>
        </View>
    )
}

const styles = StyleSheet.create({
    container: {
        width: 320,
        borderLeftWidth: 1,
    },
    header: {
        flexDirection: 'row',
        alignItems: 'flex-start',
        justifyContent: 'space-between',
        paddingHorizontal: 16,
        paddingVertical: 12,
        borderBottomWidth: 1,
        gap: 8,
    },
    headerTitle: {
        fontSize: 16,
        fontWeight: '600',
        flex: 1,
    },
    closeButton: {
        padding: 4,
    },
    iconArea: {
        alignItems: 'center',
        paddingVertical: 24,
    },
    tabs: {
        flexDirection: 'row',
        borderBottomWidth: 1,
    },
    tab: {
        flex: 1,
        alignItems: 'center',
        paddingVertical: 10,
    },
    tabText: {
        fontSize: 14,
        fontWeight: '500',
    },
    content: {
        padding: 16,
    },
    section: {
        gap: 8,
    },
    sectionTitle: {
        fontSize: 14,
        fontWeight: '600',
        marginBottom: 4,
    },
    accessRow: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 8,
    },
    avatarCircle: {
        width: 28,
        height: 28,
        borderRadius: 14,
        alignItems: 'center',
        justifyContent: 'center',
    },
    avatarText: {
        fontSize: 12,
        fontWeight: '600',
    },
    accessText: {
        fontSize: 13,
    },
    divider: {
        height: 1,
        marginVertical: 16,
    },
    detailRow: {
        flexDirection: 'row',
        paddingVertical: 4,
    },
    detailLabel: {
        fontSize: 13,
        width: 80,
    },
    detailValue: {
        fontSize: 13,
        flex: 1,
    },
    placeholderText: {
        fontSize: 14,
        textAlign: 'center',
        paddingVertical: 24,
    },
    versionRow: {
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'space-between',
        paddingVertical: 10,
        borderBottomWidth: 1,
    },
    versionInfo: {
        flex: 1,
        gap: 2,
    },
    versionNumber: {
        fontSize: 13,
        fontWeight: '500',
    },
    versionMeta: {
        fontSize: 12,
    },
    versionActions: {
        flexDirection: 'row',
        gap: 8,
    },
    versionActionButton: {
        padding: 4,
    },
})
