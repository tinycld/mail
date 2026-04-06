import { X } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import type { DriveItem } from '../types'
import { getFileIcon } from './file-icons'
import { formatBytes, formatDate } from './format-utils'

interface DetailPanelProps {
    isVisible: boolean
    item: DriveItem | undefined
    onClose: () => void
}

export function DetailPanel({ isVisible, item, onClose }: DetailPanelProps) {
    if (!isVisible || !item) return null

    return <DetailPanelContent item={item} onClose={onClose} />
}

function DetailPanelContent({ item, onClose }: { item: DriveItem; onClose: () => void }) {
    const theme = useTheme()
    const [activeTab, setActiveTab] = useState<'details' | 'activity'>('details')
    const { icon: FileIcon, color: iconColor } = getFileIcon(item.type, theme.color8.val)

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
                <FileIcon size={64} color={iconColor} />
            </View>

            <View style={[styles.tabs, { borderBottomColor: theme.borderColor.val }]}>
                <Pressable
                    style={[
                        styles.tab,
                        activeTab === 'details' && {
                            borderBottomColor: theme.accentColor.val,
                            borderBottomWidth: 2,
                        },
                    ]}
                    onPress={() => setActiveTab('details')}
                >
                    <Text
                        style={[
                            styles.tabText,
                            {
                                color:
                                    activeTab === 'details'
                                        ? theme.accentColor.val
                                        : theme.color8.val,
                            },
                        ]}
                    >
                        Details
                    </Text>
                </Pressable>
                <Pressable
                    style={[
                        styles.tab,
                        activeTab === 'activity' && {
                            borderBottomColor: theme.accentColor.val,
                            borderBottomWidth: 2,
                        },
                    ]}
                    onPress={() => setActiveTab('activity')}
                >
                    <Text
                        style={[
                            styles.tabText,
                            {
                                color:
                                    activeTab === 'activity'
                                        ? theme.accentColor.val
                                        : theme.color8.val,
                            },
                        ]}
                    >
                        Activity
                    </Text>
                </Pressable>
            </View>

            {activeTab === 'details' ? <DetailsContent item={item} /> : <ActivityContent />}
        </View>
    )
}

function DetailsContent({ item }: { item: DriveItem }) {
    const theme = useTheme()
    const accessText = item.shared ? `Shared with others` : 'Private to you'

    return (
        <View style={styles.content}>
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
                <DetailRow label="Modified" value={formatDate(item.modifiedDate)} />
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
})
