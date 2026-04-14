import { useLocalSearchParams, useRouter } from 'expo-router'
import { Archive, type Inbox, Send, Star, Tag, Trash2, TriangleAlert, X } from 'lucide-react-native'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ActivityIndicator, FlatList, Pressable, Text, View } from 'react-native'
import { ScreenHeader } from '~/components/ScreenHeader'
import { SwipeableRowProvider } from '~/components/SwipeableRow'
import { useBreakpoint } from '~/components/workspace/useBreakpoint'
import { mutation, useMutation } from '~/lib/mutations'
import { useOrgHref } from '~/lib/org-routes'
import { pb, queryClient } from '~/lib/pocketbase'
import { useThemeColor } from '~/lib/use-app-theme'
import { useCurrentRole } from '~/lib/use-current-role'
import { useScrollShadow } from '~/lib/use-scroll-shadow'
import { ComposeFAB } from '../components/ComposeFAB'
import { EmailListToolbar } from '../components/EmailListToolbar'
import { EmailRow } from '../components/EmailRow'
import type { ThreadListItem } from '../components/thread-list-item'
import { useCompose } from '../hooks/useComposeState'
import { useMailBulkActions } from '../hooks/useMailBulkActions'
import type { MailSearchResult } from '../hooks/useMailSearch'
import { useMailSelection } from '../hooks/useMailSelection'
import { useMailSearchState } from '../hooks/useSearchState'
import { useThreadListItems } from '../hooks/useThreadListItems'
import { useThreadListContext } from '../stores/thread-list-store'

function useQueryParams() {
    const { folder, label } = useLocalSearchParams<{ folder?: string; label?: string }>()
    const labels = label ? label.split(',').filter(Boolean) : []
    return { folder: folder ?? null, labels }
}

function EmptyState({ folderTitle, isVisible }: { folderTitle: string; isVisible: boolean }) {
    const mutedColor = useThemeColor('muted-foreground')
    if (!isVisible) return null
    return (
        <View style={{ flex: 1, alignItems: 'center', justifyContent: 'center', padding: 32 }}>
            <Text style={{ fontSize: 16, color: mutedColor }}>
                No conversations in {folderTitle}
            </Text>
        </View>
    )
}

const FOLDER_ICONS: Record<string, typeof Inbox> = {
    sent: Send,
    drafts: Archive,
    spam: TriangleAlert,
    trash: Trash2,
    starred: Star,
    archive: Archive,
}

function LabelChip({
    label,
    onDismiss,
}: {
    label: { id: string; name: string; color: string }
    onDismiss: () => void
}) {
    return (
        <View
            style={{
                flexDirection: 'row',
                alignItems: 'center',
                gap: 8,
                paddingHorizontal: 12,
                paddingVertical: 6,
                borderRadius: 8,
                borderWidth: 1,
                backgroundColor: `${label.color}14`,
                borderColor: `${label.color}30`,
            }}
        >
            <View
                style={{
                    width: 8,
                    height: 8,
                    borderRadius: 4,
                    backgroundColor: label.color,
                }}
            />
            <Text style={{ fontSize: 13, fontWeight: '600', color: label.color }}>
                {label.name}
            </Text>
            <Pressable onPress={onDismiss} hitSlop={8}>
                <X size={14} color={label.color} />
            </Pressable>
        </View>
    )
}

function ActiveViewBanner({
    isVisible,
    folder,
    labels,
    onDismissLabel,
}: {
    isVisible: boolean
    folder: string | null
    labels: { id: string; name: string; color: string }[]
    onDismissLabel: (labelId: string) => void
}) {
    const accentColor = useThemeColor('accent')
    const router = useRouter()
    const orgHref = useOrgHref()

    if (!isVisible) return null

    const isLabelView = labels.length > 0

    if (isLabelView) {
        return (
            <View
                style={{
                    flexDirection: 'row',
                    paddingHorizontal: 16,
                    paddingVertical: 8,
                    alignItems: 'center',
                    gap: 8,
                    flexWrap: 'wrap',
                }}
            >
                {labels.map(label => (
                    <LabelChip
                        key={label.id}
                        label={label}
                        onDismiss={() => onDismissLabel(label.id)}
                    />
                ))}
            </View>
        )
    }

    const displayName = (folder ?? '').charAt(0).toUpperCase() + (folder ?? '').slice(1)
    const FolderIcon = folder ? (FOLDER_ICONS[folder] ?? Tag) : Tag

    return (
        <View
            style={{
                flexDirection: 'row',
                paddingHorizontal: 16,
                paddingVertical: 8,
                alignItems: 'center',
                gap: 8,
            }}
        >
            <View
                style={{
                    flexDirection: 'row',
                    alignItems: 'center',
                    gap: 8,
                    paddingHorizontal: 12,
                    paddingVertical: 6,
                    borderRadius: 8,
                    borderWidth: 1,
                    backgroundColor: `${accentColor}14`,
                    borderColor: `${accentColor}30`,
                }}
            >
                <FolderIcon size={14} color={accentColor} />
                <Text style={{ fontSize: 13, fontWeight: '600', color: accentColor }}>
                    {displayName}
                </Text>
                <Pressable onPress={() => router.replace(orgHref('mail'))} hitSlop={8}>
                    <X size={14} color={accentColor} />
                </Pressable>
            </View>
        </View>
    )
}

function SearchResultsHeader({ total, isSearching }: { total: number; isSearching: boolean }) {
    const mutedColor = useThemeColor('muted-foreground')
    return (
        <View style={{ paddingHorizontal: 12, paddingVertical: 8 }}>
            <Text style={{ fontSize: 13, color: mutedColor }}>
                {isSearching ? 'Searching...' : `${total} result${total !== 1 ? 's' : ''}`}
            </Text>
        </View>
    )
}

function stripHtmlTags(html: string): string {
    return html.replace(/<[^>]*>/g, '')
}

function searchResultToThreadListItem(result: MailSearchResult): ThreadListItem {
    let participants: { name: string; email: string }[] = []
    try {
        participants =
            typeof result.participants === 'string'
                ? JSON.parse(result.participants)
                : (result.participants ?? [])
    } catch {
        // ignore parse errors
    }

    return {
        stateId: result.thread_id,
        threadId: result.thread_id,
        subject: result.subject,
        snippet: stripHtmlTags(result.snippet_highlight) || '',
        latestDate: result.latest_date,
        messageCount: result.message_count,
        senderName: participants[0]?.name ?? '',
        senderEmail: participants[0]?.email ?? '',
        participants,
        isRead: true,
        isStarred: false,
        labels: [],
        folder: 'search',
        hasDraft: false,
        hasAttachments: false,
    }
}

export default function MailListScreen() {
    const { folder, labels } = useQueryParams()
    const router = useRouter()
    const orgHref = useOrgHref()
    const breakpoint = useBreakpoint()
    const { userOrgId } = useCurrentRole()
    const { isScrolled, onScroll } = useScrollShadow()
    const { openDraft } = useCompose()
    const search = useMailSearchState()
    const mutedColor = useThemeColor('muted-foreground')
    const primaryColor = useThemeColor('primary')
    const _backgroundColor = useThemeColor('background')

    const {
        items,
        labels: allLabels,
        labelMap,
        draftByThread,
        threadStateCollection,
    } = useThreadListItems(userOrgId, {
        folder,
        labels,
    })

    const { setThreadIds } = useThreadListContext()
    const prevIdsRef = useRef('')
    useEffect(() => {
        const ids = items.map(i => i.threadId)
        const key = ids.join(',')
        if (key !== prevIdsRef.current) {
            prevIdsRef.current = key
            setThreadIds(ids)
        }
    }, [items, setThreadIds])

    const [isRefreshing, setIsRefreshing] = useState(false)
    const handleRefresh = useCallback(() => {
        setIsRefreshing(true)
        queryClient.invalidateQueries()
        setTimeout(() => setIsRefreshing(false), 800)
    }, [])

    const selection = useMailSelection(items, folder, labels)
    const bulkActions = useMailBulkActions(
        threadStateCollection,
        selection.selectedItems,
        selection.clearSelection
    )

    const selectedItemLabelIds = useMemo(() => {
        if (selection.selectedItems.length === 0) return new Set<string>()
        const sets = selection.selectedItems.map(item => new Set(item.labels.map(l => l.id)))
        const firstIds = Array.from(sets[0])
        const intersection = new Set<string>(firstIds.filter(id => sets.every(s => s.has(id))))
        return intersection
    }, [selection.selectedItems])

    const toggleStar = useMutation({
        mutationFn: mutation(function* ({
            stateId,
            currentStarred,
        }: {
            stateId: string
            currentStarred: boolean
        }) {
            yield threadStateCollection.update(stateId, draft => {
                draft.is_starred = !currentStarred
            })
        }),
    })

    const archiveThread = useMutation({
        mutationFn: mutation(function* ({ stateId, folder }: { stateId: string; folder: string }) {
            yield threadStateCollection.update(stateId, draft => {
                draft.folder = folder === 'archive' ? 'inbox' : 'archive'
            })
        }),
    })

    const trashThread = useMutation({
        mutationFn: mutation(function* ({ stateId, folder }: { stateId: string; folder: string }) {
            yield threadStateCollection.update(stateId, draft => {
                draft.folder = folder === 'trash' ? 'inbox' : 'trash'
            })
        }),
    })

    const toggleRead = useMutation({
        mutationFn: mutation(function* ({
            stateId,
            currentRead,
        }: {
            stateId: string
            currentRead: boolean
        }) {
            yield threadStateCollection.update(stateId, draft => {
                draft.is_read = !currentRead
            })
        }),
    })

    const handleDraftPress = useCallback(
        async (item: ThreadListItem) => {
            const draft = draftByThread.get(item.threadId)
            if (!draft) return

            let htmlBody = ''
            if (draft.body_html) {
                const url = pb.files.getURL(
                    { collectionId: 'mail_messages', id: draft.id },
                    draft.body_html
                )
                htmlBody = await fetch(url)
                    .then(r => r.text())
                    .catch(() => '')
            }

            openDraft({
                messageId: draft.id,
                threadId: item.threadId,
                subject: draft.subject ?? '',
                to: draft.recipients_to ?? [],
                cc: draft.recipients_cc ?? [],
                bcc: [],
                htmlBody,
                textBody: draft.snippet ?? '',
            })
        },
        [draftByThread, openDraft]
    )

    const searchItems = useMemo(
        () => search.results.map(searchResultToThreadListItem),
        [search.results]
    )

    const activeLabels = labels
        .map(id => labelMap.get(id))
        .filter((l): l is { id: string; name: string; color: string } => l != null)
    const folderTitle =
        activeLabels.length > 0
            ? activeLabels.map(l => l.name).join(', ')
            : (folder ?? 'inbox').charAt(0).toUpperCase() + (folder ?? 'inbox').slice(1)

    const isNonDefaultView = labels.length > 0 || (!!folder && folder !== 'inbox')

    const dismissLabel = useCallback(
        (labelId: string) => {
            const remaining = labels.filter(id => id !== labelId)
            if (remaining.length === 0) {
                router.replace(orgHref('mail'))
            } else {
                router.replace(orgHref('mail', { label: remaining.join(',') }))
            }
        },
        [labels, router, orgHref]
    )

    const isMobile = breakpoint === 'mobile'

    if (search.isActive) {
        return (
            <View style={{ flex: 1 }}>
                <SearchResultsHeader total={search.total} isSearching={search.isSearching} />
                {search.isSearching && searchItems.length === 0 ? (
                    <View style={{ flex: 1, alignItems: 'center', justifyContent: 'center' }}>
                        <ActivityIndicator size="large" color={primaryColor} />
                    </View>
                ) : searchItems.length === 0 ? (
                    <View
                        style={{
                            flex: 1,
                            alignItems: 'center',
                            justifyContent: 'center',
                            padding: 32,
                        }}
                    >
                        <Text style={{ fontSize: 16, color: mutedColor }}>No results found</Text>
                    </View>
                ) : (
                    <SwipeableRowProvider>
                        <FlatList
                            data={searchItems}
                            keyExtractor={item => item.threadId}
                            renderItem={({ item }) => <EmailRow email={item} isMobile={isMobile} />}
                        />
                    </SwipeableRowProvider>
                )}
            </View>
        )
    }

    const isEmpty = items.length === 0

    return (
        <View style={{ flex: 1 }}>
            <ActiveViewBanner
                isVisible={isNonDefaultView}
                folder={folder}
                labels={activeLabels}
                onDismissLabel={dismissLabel}
            />
            <ScreenHeader isScrolled={isScrolled}>
                <EmailListToolbar
                    emailCount={items.length}
                    hasSelection={selection.hasSelection}
                    selectedCount={selection.selectedCount}
                    allSelected={selection.allSelected}
                    someSelected={selection.someSelected}
                    allSelectedRead={selection.allSelectedRead}
                    allSelectedStarred={selection.allSelectedStarred}
                    labels={allLabels}
                    selectedItemLabelIds={selectedItemLabelIds}
                    onToggleAll={selection.toggleAll}
                    onArchive={() => bulkActions.archiveSelected.mutate()}
                    onSpam={() => bulkActions.spamSelected.mutate()}
                    onTrash={() => bulkActions.trashSelected.mutate()}
                    onToggleRead={markAsRead =>
                        bulkActions.toggleReadSelected.mutate({ markAsRead })
                    }
                    onMove={folder => bulkActions.moveSelected.mutate(folder)}
                    onToggleStar={star => bulkActions.toggleStarSelected.mutate({ star })}
                    onUpdateLabel={(labelId, add) =>
                        bulkActions.updateLabelsSelected.mutate({ labelId, add })
                    }
                    onRefresh={handleRefresh}
                    isRefreshing={isRefreshing}
                />
            </ScreenHeader>
            <EmptyState folderTitle={folderTitle} isVisible={isEmpty} />
            {isEmpty ? null : (
                <SwipeableRowProvider>
                    <FlatList
                        data={items}
                        keyExtractor={item => item.stateId}
                        onScroll={onScroll}
                        scrollEventThrottle={16}
                        renderItem={({ item, index }) => (
                            <EmailRow
                                email={item}
                                isMobile={isMobile}
                                index={index}
                                isSelected={selection.selectedIds.has(item.stateId)}
                                onToggleSelect={() => selection.toggle(item.stateId)}
                                onToggleStar={() =>
                                    toggleStar.mutate({
                                        stateId: item.stateId,
                                        currentStarred: item.isStarred,
                                    })
                                }
                                onPress={item.hasDraft ? () => handleDraftPress(item) : undefined}
                                onArchive={() =>
                                    archiveThread.mutate({
                                        stateId: item.stateId,
                                        folder: item.folder,
                                    })
                                }
                                onTrash={() =>
                                    trashThread.mutate({
                                        stateId: item.stateId,
                                        folder: item.folder,
                                    })
                                }
                                onToggleRead={() =>
                                    toggleRead.mutate({
                                        stateId: item.stateId,
                                        currentRead: item.isRead,
                                    })
                                }
                            />
                        )}
                    />
                </SwipeableRowProvider>
            )}
            <ComposeFAB isVisible={isMobile} />
        </View>
    )
}
