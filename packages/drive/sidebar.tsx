import {
    ChevronDown,
    ChevronRight,
    Clock,
    Folder,
    HardDrive,
    Plus,
    Star,
    Trash2,
    UserPlus,
    Users,
} from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import { SidebarDivider, SidebarItem, SidebarNav } from '~/components/sidebar-primitives'
import { useDrive } from './hooks/useDrive'
import { getFolderTree } from './mock-data'
import type { FolderTreeNode } from './types'

interface DriveSidebarProps {
    isCollapsed: boolean
}

export default function DriveSidebar(_props: DriveSidebarProps) {
    const theme = useTheme()
    const { activeSection, currentFolderId, navigateToFolder, navigateToSection } = useDrive()
    const [expandedIds, setExpandedIds] = useState<Set<string>>(() => new Set())
    const folderTree = getFolderTree()

    const toggleExpand = (id: string) => {
        setExpandedIds(prev => {
            const next = new Set(prev)
            if (next.has(id)) {
                next.delete(id)
            } else {
                next.add(id)
            }
            return next
        })
    }

    const handleFolderPress = (id: string) => {
        navigateToFolder(id)
        setExpandedIds(prev => {
            const next = new Set(prev)
            next.add(id)
            return next
        })
    }

    return (
        <SidebarNav>
            <View style={styles.newWrapper}>
                <Pressable
                    style={[styles.newButton, { backgroundColor: theme.accentBackground.val }]}
                >
                    <Plus size={16} color={theme.accentColor.val} />
                    <Text style={[styles.newText, { color: theme.accentColor.val }]}>New</Text>
                </Pressable>
            </View>

            <SidebarItem
                label="My Drive"
                icon={HardDrive}
                isActive={activeSection === 'my-drive' && currentFolderId === null}
                onPress={() => navigateToSection('my-drive')}
            />

            <FolderTree
                nodes={folderTree}
                expandedIds={expandedIds}
                selectedFolderId={currentFolderId}
                onToggle={toggleExpand}
                onSelect={handleFolderPress}
                depth={1}
            />

            <SidebarItem
                label="Shared drives"
                icon={Users}
                isActive={activeSection === 'shared-drives'}
                onPress={() => navigateToSection('shared-drives')}
            />

            <SidebarDivider />

            <SidebarItem
                label="Shared with me"
                icon={UserPlus}
                isActive={activeSection === 'shared-with-me'}
                onPress={() => navigateToSection('shared-with-me')}
            />
            <SidebarItem
                label="Recent"
                icon={Clock}
                isActive={activeSection === 'recent'}
                onPress={() => navigateToSection('recent')}
            />
            <SidebarItem
                label="Starred"
                icon={Star}
                isActive={activeSection === 'starred'}
                onPress={() => navigateToSection('starred')}
            />

            <SidebarDivider />

            <SidebarItem
                label="Trash"
                icon={Trash2}
                isActive={activeSection === 'trash'}
                onPress={() => navigateToSection('trash')}
            />

            <SidebarDivider />

            <StorageBar usedGB={7.5} totalGB={15} />
        </SidebarNav>
    )
}

interface FolderTreeProps {
    nodes: FolderTreeNode[]
    expandedIds: Set<string>
    selectedFolderId: string | null
    onToggle: (id: string) => void
    onSelect: (id: string) => void
    depth: number
}

function FolderTree({
    nodes,
    expandedIds,
    selectedFolderId,
    onToggle,
    onSelect,
    depth,
}: FolderTreeProps) {
    if (nodes.length === 0) return null

    return (
        <View>
            {nodes.map(node => (
                <FolderTreeItem
                    key={node.item.id}
                    node={node}
                    expandedIds={expandedIds}
                    selectedFolderId={selectedFolderId}
                    onToggle={onToggle}
                    onSelect={onSelect}
                    depth={depth}
                />
            ))}
        </View>
    )
}

interface FolderTreeItemProps {
    node: FolderTreeNode
    expandedIds: Set<string>
    selectedFolderId: string | null
    onToggle: (id: string) => void
    onSelect: (id: string) => void
    depth: number
}

function FolderTreeItem({
    node,
    expandedIds,
    selectedFolderId,
    onToggle,
    onSelect,
    depth,
}: FolderTreeItemProps) {
    const theme = useTheme()
    const isExpanded = expandedIds.has(node.item.id)
    const isSelected = selectedFolderId === node.item.id
    const hasChildren = node.children.length > 0
    const ChevronIcon = isExpanded ? ChevronDown : ChevronRight

    return (
        <View key={node.item.id}>
            <Pressable
                style={[
                    styles.treeItem,
                    { paddingLeft: depth * 16 },
                    isSelected && { backgroundColor: `${theme.activeIndicator.val}18` },
                ]}
                onPress={() => onSelect(node.item.id)}
            >
                {hasChildren ? (
                    <Pressable onPress={() => onToggle(node.item.id)} style={styles.chevron}>
                        <ChevronIcon size={14} color={theme.color8.val} />
                    </Pressable>
                ) : (
                    <View style={styles.chevron} />
                )}
                <Folder
                    size={16}
                    color={isSelected ? theme.activeIndicator.val : theme.color8.val}
                />
                <Text
                    style={[
                        styles.treeLabel,
                        { color: isSelected ? theme.activeIndicator.val : theme.color.val },
                        isSelected && styles.treeLabelActive,
                    ]}
                    numberOfLines={1}
                >
                    {node.item.name}
                </Text>
            </Pressable>
            {isExpanded && node.children.length > 0 && (
                <FolderTree
                    nodes={node.children}
                    expandedIds={expandedIds}
                    selectedFolderId={selectedFolderId}
                    onToggle={onToggle}
                    onSelect={onSelect}
                    depth={depth + 1}
                />
            )}
        </View>
    )
}

function StorageBar({ usedGB, totalGB }: { usedGB: number; totalGB: number }) {
    const theme = useTheme()
    const percentage = (usedGB / totalGB) * 100

    return (
        <View style={styles.storageContainer}>
            <View style={[styles.storageTrack, { backgroundColor: `${theme.color8.val}20` }]}>
                <View
                    style={[
                        styles.storageFill,
                        { width: `${percentage}%`, backgroundColor: theme.accentBackground.val },
                    ]}
                />
            </View>
            <Text style={[styles.storageText, { color: theme.color8.val }]}>
                {usedGB} GB of {totalGB} GB used
            </Text>
        </View>
    )
}

const styles = StyleSheet.create({
    newWrapper: {
        paddingHorizontal: 12,
        paddingVertical: 8,
    },
    newButton: {
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'center',
        gap: 8,
        paddingHorizontal: 20,
        paddingVertical: 12,
        borderRadius: 24,
    },
    newText: {
        fontSize: 14,
        fontWeight: '600',
    },
    treeItem: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 6,
        paddingVertical: 6,
        paddingRight: 12,
        borderRadius: 8,
    },
    chevron: {
        width: 18,
        alignItems: 'center',
        justifyContent: 'center',
    },
    treeLabel: {
        fontSize: 13,
        flex: 1,
    },
    treeLabelActive: {
        fontWeight: '600',
    },
    storageContainer: {
        paddingHorizontal: 12,
        paddingVertical: 8,
        gap: 6,
    },
    storageTrack: {
        height: 4,
        borderRadius: 2,
        overflow: 'hidden',
    },
    storageFill: {
        height: '100%',
        borderRadius: 2,
    },
    storageText: {
        fontSize: 12,
    },
})
