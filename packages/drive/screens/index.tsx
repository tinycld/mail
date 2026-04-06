import { Star } from 'lucide-react-native'
import { Pressable, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import { getFileIcon } from '../components/file-icons'
import { formatBytes, formatDate } from '../components/format-utils'
import { useDrive } from '../hooks/useDrive'
import type { DriveItem } from '../types'

export default function DriveScreen() {
    const { viewMode, currentItems } = useDrive()

    if (currentItems.length === 0) {
        return <EmptyState />
    }

    if (viewMode === 'grid') {
        return <GridView items={currentItems} />
    }

    return <ListView items={currentItems} />
}

function EmptyState() {
    const theme = useTheme()
    return (
        <View style={styles.emptyContainer}>
            <Text style={[styles.emptyText, { color: theme.color8.val }]}>
                No files in this location
            </Text>
        </View>
    )
}

function ListView({ items }: { items: DriveItem[] }) {
    const theme = useTheme()
    const folders = items.filter(i => i.type === 'folder')
    const files = items.filter(i => i.type !== 'folder')

    return (
        <View style={styles.listContainer}>
            <View style={[styles.listHeader, { borderBottomColor: theme.borderColor.val }]}>
                <Text style={[styles.headerText, { color: theme.color8.val, flex: 3 }]}>Name</Text>
                <Text style={[styles.headerText, { color: theme.color8.val, flex: 2 }]}>Owner</Text>
                <Text style={[styles.headerText, { color: theme.color8.val, flex: 2 }]}>
                    Date modified
                </Text>
                <Text style={[styles.headerText, { color: theme.color8.val, flex: 1 }]}>
                    File size
                </Text>
            </View>
            {folders.map(item => (
                <DriveListRow key={item.id} item={item} />
            ))}
            {files.map(item => (
                <DriveListRow key={item.id} item={item} />
            ))}
        </View>
    )
}

function DriveListRow({ item }: { item: DriveItem }) {
    const theme = useTheme()
    const { selectedItemId, openItem } = useDrive()
    const isSelected = selectedItemId === item.id
    const { icon: FileIcon, color: iconColor } = getFileIcon(item.type, theme.color8.val)

    return (
        <Pressable
            onPress={() => openItem(item)}
            style={[
                styles.listRow,
                { borderBottomColor: theme.borderColor.val },
                isSelected && { backgroundColor: `${theme.activeIndicator.val}12` },
            ]}
        >
            <View style={[styles.nameCell, { flex: 3 }]}>
                <FileIcon size={20} color={iconColor} />
                <Text style={[styles.nameText, { color: theme.color.val }]} numberOfLines={1}>
                    {item.name}
                </Text>
                {item.starred && (
                    <Star size={14} color={theme.yellow8.val} fill={theme.yellow8.val} />
                )}
            </View>
            <Text style={[styles.cellText, { color: theme.color8.val, flex: 2 }]} numberOfLines={1}>
                {item.owner}
            </Text>
            <Text style={[styles.cellText, { color: theme.color8.val, flex: 2 }]}>
                {formatDate(item.modifiedDate)}
            </Text>
            <Text style={[styles.cellText, { color: theme.color8.val, flex: 1 }]}>
                {item.type === 'folder' ? '—' : formatBytes(item.size)}
            </Text>
        </Pressable>
    )
}

function GridView({ items }: { items: DriveItem[] }) {
    const folders = items.filter(i => i.type === 'folder')
    const files = items.filter(i => i.type !== 'folder')

    return (
        <View style={styles.gridContainer}>
            {folders.length > 0 && (
                <View style={styles.gridSection}>
                    <GridSectionHeader title="Folders" />
                    <View style={styles.gridWrap}>
                        {folders.map(item => (
                            <FolderGridCard key={item.id} item={item} />
                        ))}
                    </View>
                </View>
            )}
            {files.length > 0 && (
                <View style={styles.gridSection}>
                    <GridSectionHeader title="Files" />
                    <View style={styles.gridWrap}>
                        {files.map(item => (
                            <FileGridCard key={item.id} item={item} />
                        ))}
                    </View>
                </View>
            )}
        </View>
    )
}

function GridSectionHeader({ title }: { title: string }) {
    const theme = useTheme()
    return <Text style={[styles.gridSectionTitle, { color: theme.color8.val }]}>{title}</Text>
}

function FolderGridCard({ item }: { item: DriveItem }) {
    const theme = useTheme()
    const { selectedItemId, openItem } = useDrive()
    const isSelected = selectedItemId === item.id
    const { icon: FileIcon, color: iconColor } = getFileIcon(item.type, theme.color8.val)

    return (
        <Pressable
            onPress={() => openItem(item)}
            style={[
                styles.folderCard,
                { borderColor: theme.borderColor.val },
                isSelected && { borderColor: theme.activeIndicator.val, borderWidth: 2 },
            ]}
        >
            <FileIcon size={20} color={iconColor} />
            <Text style={[styles.cardName, { color: theme.color.val }]} numberOfLines={1}>
                {item.name}
            </Text>
        </Pressable>
    )
}

function FileGridCard({ item }: { item: DriveItem }) {
    const theme = useTheme()
    const { selectedItemId, openItem } = useDrive()
    const isSelected = selectedItemId === item.id
    const { icon: FileIcon, color: iconColor } = getFileIcon(item.type, theme.color8.val)

    return (
        <Pressable
            onPress={() => openItem(item)}
            style={[
                styles.fileCard,
                { borderColor: theme.borderColor.val },
                isSelected && { borderColor: theme.activeIndicator.val, borderWidth: 2 },
            ]}
        >
            <View style={[styles.fileCardHeader, { borderBottomColor: theme.borderColor.val }]}>
                <FileIcon size={18} color={iconColor} />
                <Text style={[styles.cardName, { color: theme.color.val }]} numberOfLines={1}>
                    {item.name}
                </Text>
            </View>
            <View style={[styles.fileCardBody, { backgroundColor: `${theme.color8.val}08` }]}>
                <FileIcon size={40} color={iconColor} />
            </View>
        </Pressable>
    )
}

const styles = StyleSheet.create({
    emptyContainer: {
        flex: 1,
        alignItems: 'center',
        justifyContent: 'center',
        padding: 24,
    },
    emptyText: {
        fontSize: 15,
    },
    listContainer: {
        flex: 1,
        paddingHorizontal: 16,
    },
    listHeader: {
        flexDirection: 'row',
        paddingHorizontal: 12,
        paddingVertical: 8,
        borderBottomWidth: 1,
    },
    headerText: {
        fontSize: 12,
        fontWeight: '600',
        textTransform: 'uppercase',
        letterSpacing: 0.5,
    },
    listRow: {
        flexDirection: 'row',
        alignItems: 'center',
        paddingHorizontal: 12,
        paddingVertical: 10,
        borderBottomWidth: 1,
    },
    nameCell: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 10,
    },
    nameText: {
        fontSize: 14,
        fontWeight: '500',
        flex: 1,
    },
    cellText: {
        fontSize: 13,
    },
    gridContainer: {
        flex: 1,
        padding: 16,
    },
    gridSection: {
        marginBottom: 20,
    },
    gridSectionTitle: {
        fontSize: 13,
        fontWeight: '600',
        textTransform: 'uppercase',
        letterSpacing: 0.5,
        marginBottom: 10,
    },
    gridWrap: {
        flexDirection: 'row',
        flexWrap: 'wrap',
        gap: 12,
    },
    folderCard: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 10,
        width: 180,
        paddingHorizontal: 12,
        paddingVertical: 10,
        borderWidth: 1,
        borderRadius: 8,
    },
    fileCard: {
        width: 180,
        borderWidth: 1,
        borderRadius: 8,
        overflow: 'hidden',
    },
    fileCardHeader: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 8,
        paddingHorizontal: 10,
        paddingVertical: 8,
        borderBottomWidth: 1,
    },
    fileCardBody: {
        height: 120,
        alignItems: 'center',
        justifyContent: 'center',
    },
    cardName: {
        fontSize: 13,
        fontWeight: '500',
        flex: 1,
    },
})
