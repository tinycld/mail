import { useLocalSearchParams } from 'expo-router'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { FlatList } from 'react-native'
import { SizableText, Spinner, YStack } from 'tamagui'
import { ScreenHeader } from '~/components/ScreenHeader'
import { useBreakpoint } from '~/components/workspace/useBreakpoint'
import { useMutation } from '~/lib/mutations'
import { pb, queryClient } from '~/lib/pocketbase'
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
import { useThreadListContext } from './_layout'

function useQueryParams() {
    const { folder, label } = useLocalSearchParams<{ folder?: string; label?: string }>()
    return { folder: folder ?? null, label: label ?? null }
}

function EmptyState({ folderTitle, isVisible }: { folderTitle: string; isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <YStack flex={1} alignItems="center" justifyContent="center" padding="$8">
            <SizableText size="$4" color="$color8">
                No conversations in {folderTitle}
            </SizableText>
        </YStack>
    )
}

function SearchResultsHeader({ total, isSearching }: { total: number; isSearching: boolean }) {
    return (
        <YStack paddingHorizontal="$3" paddingVertical="$2">
            <SizableText size="$3" color="$color8">
                {isSearching ? 'Searching...' : `${total} result${total !== 1 ? 's' : ''}`}
            </SizableText>
        </YStack>
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
    const { folder, label } = useQueryParams()
    const breakpoint = useBreakpoint()
    const { userOrgId } = useCurrentRole()
    const { isScrolled, onScroll } = useScrollShadow()
    const { openDraft } = useCompose()
    const search = useMailSearchState()

    const { items, labels, draftByThread, threadStateCollection } = useThreadListItems(userOrgId, {
        folder,
        label,
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

    const selection = useMailSelection(items, folder, label)
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
        mutationFn: function* ({
            stateId,
            currentStarred,
        }: {
            stateId: string
            currentStarred: boolean
        }) {
            yield threadStateCollection.update(stateId, draft => {
                draft.is_starred = !currentStarred
            })
        },
    })

    const archiveThread = useMutation({
        mutationFn: function* ({ stateId, folder }: { stateId: string; folder: string }) {
            yield threadStateCollection.update(stateId, draft => {
                draft.folder = folder === 'archive' ? 'inbox' : 'archive'
            })
        },
    })

    const trashThread = useMutation({
        mutationFn: function* ({ stateId, folder }: { stateId: string; folder: string }) {
            yield threadStateCollection.update(stateId, draft => {
                draft.folder = folder === 'trash' ? 'inbox' : 'trash'
            })
        },
    })

    const toggleRead = useMutation({
        mutationFn: function* ({
            stateId,
            currentRead,
        }: {
            stateId: string
            currentRead: boolean
        }) {
            yield threadStateCollection.update(stateId, draft => {
                draft.is_read = !currentRead
            })
        },
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

    const folderTitle = label
        ? 'Label'
        : (folder ?? 'inbox').charAt(0).toUpperCase() + (folder ?? 'inbox').slice(1)

    const isMobile = breakpoint === 'mobile'

    if (search.isActive) {
        return (
            <YStack flex={1}>
                <SearchResultsHeader total={search.total} isSearching={search.isSearching} />
                {search.isSearching && searchItems.length === 0 ? (
                    <YStack flex={1} alignItems="center" justifyContent="center">
                        <Spinner size="large" color="$accentColor" />
                    </YStack>
                ) : searchItems.length === 0 ? (
                    <YStack flex={1} alignItems="center" justifyContent="center" padding="$8">
                        <SizableText size="$4" color="$color8">
                            No results found
                        </SizableText>
                    </YStack>
                ) : (
                    <FlatList
                        data={searchItems}
                        keyExtractor={item => item.threadId}
                        renderItem={({ item }) => <EmailRow email={item} isMobile={isMobile} />}
                    />
                )}
            </YStack>
        )
    }

    const isEmpty = items.length === 0

    return (
        <YStack flex={1}>
            <ScreenHeader isScrolled={isScrolled}>
                <EmailListToolbar
                    emailCount={items.length}
                    hasSelection={selection.hasSelection}
                    selectedCount={selection.selectedCount}
                    allSelected={selection.allSelected}
                    someSelected={selection.someSelected}
                    allSelectedRead={selection.allSelectedRead}
                    allSelectedStarred={selection.allSelectedStarred}
                    labels={labels}
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
                                archiveThread.mutate({ stateId: item.stateId, folder: item.folder })
                            }
                            onTrash={() =>
                                trashThread.mutate({ stateId: item.stateId, folder: item.folder })
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
            )}
            <ComposeFAB isVisible={isMobile} />
        </YStack>
    )
}
