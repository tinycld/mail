import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useParams } from 'one'
import { useCallback, useMemo } from 'react'
import { FlatList } from 'react-native'
import { SizableText, Spinner, YStack } from 'tamagui'
import { useBreakpoint } from '~/components/workspace/useBreakpoint'
import { useMutation } from '~/lib/mutations'
import { pb, useStore } from '~/lib/pocketbase'
import { useCurrentRole } from '~/lib/use-current-role'
import { ComposeFAB } from '../components/ComposeFAB'
import { EmailListToolbar } from '../components/EmailListToolbar'
import { EmailRow } from '../components/EmailRow'
import type { ThreadListItem } from '../components/thread-list-item'
import { toThreadListItem } from '../components/thread-list-item'
import { useCompose } from '../hooks/useComposeState'
import { useMailBulkActions } from '../hooks/useMailBulkActions'
import type { MailSearchResult } from '../hooks/useMailSearch'
import { useMailSelection } from '../hooks/useMailSelection'
import { useMailSearchState } from '../hooks/useSearchState'
import type { MailMessages } from '../types'

function useQueryParams() {
    const { folder, label } = useParams<{ folder?: string; label?: string }>()
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
    const { openDraft } = useCompose()
    const search = useMailSearchState()

    const [threadStateCollection, threadsCollection, labelsCollection, messagesCollection] =
        useStore('mail_thread_state', 'mail_threads', 'mail_labels', 'mail_messages')

    const { data: threadStates } = useLiveQuery(
        query =>
            query
                .from({ mail_thread_state: threadStateCollection })
                .where(({ mail_thread_state }) => eq(mail_thread_state.user_org, userOrgId))
                .orderBy(({ mail_thread_state }) => mail_thread_state.updated, 'desc'),
        [userOrgId]
    )

    const { data: threads } = useLiveQuery(
        query => query.from({ mail_threads: threadsCollection }),
        []
    )

    const { data: allLabels } = useLiveQuery(
        query => query.from({ mail_labels: labelsCollection }),
        []
    )

    const { data: draftMessages } = useLiveQuery(
        query =>
            query
                .from({ mail_messages: messagesCollection })
                .where(({ mail_messages }) => eq(mail_messages.delivery_status, 'draft')),
        []
    )

    const draftByThread = useMemo(() => {
        const map = new Map<string, MailMessages>()
        for (const msg of (draftMessages ?? []) as MailMessages[]) {
            map.set(msg.thread, msg)
        }
        return map
    }, [draftMessages])

    const { data: attachmentMessages } = useLiveQuery(
        query =>
            query
                .from({ mail_messages: messagesCollection })
                .where(({ mail_messages }) => eq(mail_messages.has_attachments, true)),
        []
    )

    const threadsWithAttachments = useMemo(() => {
        const set = new Set<string>()
        for (const msg of (attachmentMessages ?? []) as MailMessages[]) {
            set.add(msg.thread)
        }
        return set
    }, [attachmentMessages])

    const threadMap = useMemo(() => {
        const map = new Map<string, (typeof threads)[number]>()
        for (const t of threads ?? []) {
            map.set(t.id, t)
        }
        return map
    }, [threads])

    const labelMap = useMemo(() => {
        const map = new Map<string, { id: string; name: string; color: string }>()
        for (const l of allLabels ?? []) {
            map.set(l.id, l)
        }
        return map
    }, [allLabels])

    const items: ThreadListItem[] = useMemo(() => {
        if (!threadStates) return []

        const mapped = threadStates.map(state => {
            const thread = threadMap.get(state.thread)
            const labelIds: string[] = state.labels ?? []
            const stateLabels = labelIds
                .map((lid: string) => labelMap.get(lid))
                .filter((l): l is { id: string; name: string; color: string } => l != null)
            const hasDraft = draftByThread.has(state.thread)
            const hasAttachments = threadsWithAttachments.has(state.thread)
            return toThreadListItem(state, thread, stateLabels, hasDraft, hasAttachments)
        })

        if (label) {
            return mapped.filter(item => item.labels.some(l => l.id === label))
        }

        const activeFolder = folder ?? 'inbox'
        if (activeFolder === 'starred') {
            return mapped.filter(item => item.isStarred)
        }

        return mapped.filter(item => item.folder === activeFolder)
    }, [threadStates, threadMap, labelMap, draftByThread, threadsWithAttachments, folder, label])

    const selection = useMailSelection(items, folder, label)
    const bulkActions = useMailBulkActions(
        threadStateCollection,
        selection.selectedItems,
        selection.clearSelection
    )

    const allLabelsList = useMemo(() => {
        return Array.from(labelMap.values())
    }, [labelMap])

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
            <EmailListToolbar
                emailCount={items.length}
                hasSelection={selection.hasSelection}
                selectedCount={selection.selectedCount}
                allSelected={selection.allSelected}
                someSelected={selection.someSelected}
                allSelectedRead={selection.allSelectedRead}
                allSelectedStarred={selection.allSelectedStarred}
                labels={allLabelsList}
                selectedItemLabelIds={selectedItemLabelIds}
                onToggleAll={selection.toggleAll}
                onArchive={() => bulkActions.archiveSelected.mutate()}
                onSpam={() => bulkActions.spamSelected.mutate()}
                onTrash={() => bulkActions.trashSelected.mutate()}
                onToggleRead={markAsRead => bulkActions.toggleReadSelected.mutate({ markAsRead })}
                onMove={folder => bulkActions.moveSelected.mutate(folder)}
                onToggleStar={star => bulkActions.toggleStarSelected.mutate({ star })}
                onUpdateLabel={(labelId, add) =>
                    bulkActions.updateLabelsSelected.mutate({ labelId, add })
                }
            />
            <EmptyState folderTitle={folderTitle} isVisible={isEmpty} />
            {isEmpty ? null : (
                <FlatList
                    data={items}
                    keyExtractor={item => item.stateId}
                    renderItem={({ item }) => (
                        <EmailRow
                            email={item}
                            isMobile={isMobile}
                            isSelected={selection.selectedIds.has(item.stateId)}
                            onToggleSelect={() => selection.toggle(item.stateId)}
                            onToggleStar={() =>
                                toggleStar.mutate({
                                    stateId: item.stateId,
                                    currentStarred: item.isStarred,
                                })
                            }
                            onPress={item.hasDraft ? () => handleDraftPress(item) : undefined}
                        />
                    )}
                />
            )}
            <ComposeFAB isVisible={isMobile} />
        </YStack>
    )
}
