import { and, eq } from '@tanstack/db'
import { ScreenHeader } from '@tinycld/core/components/ScreenHeader'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { type Shortcut, useRegisterShortcut, useShortcutScope } from '@tinycld/core/lib/shortcuts'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import { useScrollShadow } from '@tinycld/core/lib/use-scroll-shadow'
import { useLocalSearchParams, useRouter } from 'expo-router'
import { useCallback, useMemo, useRef, useState } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'
import { EmailAttachments } from '../components/EmailAttachments'
import { EmailBody } from '../components/EmailBody'
import { EmailDetailToolbar } from '../components/EmailDetailToolbar'
import { MessageHeader, ThreadSubjectHeader } from '../components/EmailHeader'
import { InlineReply } from '../components/InlineReply'
import { NotFoundState } from '../components/NotFoundState'
import { filterOwnAddresses, pickDefaultFrom } from '../hooks/defaultFromIdentity'
import { useCompose } from '../hooks/useComposeState'
import { useLabels, useThreadLabels } from '../hooks/useLabels'
import { useMailboxes } from '../hooks/useMailboxes'
import { useSendableIdentities } from '../hooks/useSendableIdentities'
import { useThreadActions } from '../hooks/useThreadActions'
import { useThreadNavigation } from '../hooks/useThreadNavigation'
import { useThreadListContext } from '../stores/thread-list-store'
import type { MailMessages } from '../types'

function useAutoMarkAsRead(
    // biome-ignore lint/suspicious/noExplicitAny: pbtsdb collection type is deeply generic
    threadStateCollection: any,
    threadState: { id: string; is_read: boolean } | undefined,
    threadId: string
) {
    const markedRef = useRef<string | null>(null)
    const markAsRead = useMutation({
        mutationFn: mutation(function* (stateId: string) {
            yield threadStateCollection.update(stateId, (draft) => {
                draft.is_read = true
            })
        }),
    })
    if (threadState && !threadState.is_read && markedRef.current !== threadId) {
        markedRef.current = threadId
        markAsRead.mutate(threadState.id)
    }
}

export default function MailDetailScreen() {
    const { id = '' } = useLocalSearchParams<{ id: string }>()
    const router = useRouter()
    const { openReply } = useCompose()
    const _mutedColor = useThemeColor('muted-foreground')
    const _borderColor = useThemeColor('border')
    const backgroundColor = useThemeColor('background')

    const [threadStateCollection, messagesCollection, threadsCollection] = useStore(
        'mail_thread_state',
        'mail_messages',
        'mail_threads'
    )

    const { data: threadStates } = useOrgLiveQuery(
        (query, { userOrgId }) =>
            query
                .from({ mail_thread_state: threadStateCollection })
                .where(({ mail_thread_state }) =>
                    and(eq(mail_thread_state.thread, id), eq(mail_thread_state.user_org, userOrgId))
                ),
        [id]
    )

    const threadState = threadStates?.[0]

    const { data: threads } = useOrgLiveQuery(
        (query) => query.from({ mail_threads: threadsCollection }).where(({ mail_threads }) => eq(mail_threads.id, id)),
        [id]
    )
    const thread = threads?.[0]

    const { personal, shared } = useMailboxes()
    const identities = useSendableIdentities()
    const userMailboxIds = new Set<string>()
    if (personal) userMailboxIds.add(personal.id)
    for (const mb of shared) userMailboxIds.add(mb.id)
    const hasAccess = !thread || userMailboxIds.has(thread.mailbox)

    const { data: messages } = useOrgLiveQuery(
        (query) =>
            query
                .from({ mail_messages: messagesCollection })
                .where(({ mail_messages }) => eq(mail_messages.thread, id))
                .orderBy(({ mail_messages }) => mail_messages.date, 'asc'),
        [id]
    )

    const { labels: allLabels } = useLabels()
    const { labels, labelIds: threadLabelIds } = useThreadLabels(threadState?.id ?? '')

    useAutoMarkAsRead(threadStateCollection, threadState, id)

    const orgHref = useOrgHref()
    const initialFolderRef = useRef<string | null>(null)
    if (threadState?.folder && !initialFolderRef.current) {
        initialFolderRef.current = threadState.folder
    }
    const navigateBack = useCallback(() => {
        if (router.canGoBack()) {
            router.back()
        } else {
            router.replace(orgHref('mail', { folder: initialFolderRef.current ?? 'inbox' }))
        }
    }, [router, orgHref])

    useShortcutScope('thread')
    const closeShortcut = useMemo<Shortcut>(
        () => ({
            id: 'mail.thread.close',
            keys: 'Escape',
            scope: 'thread',
            group: 'Mail',
            description: 'Close conversation',
            run: navigateBack,
        }),
        [navigateBack]
    )
    useRegisterShortcut(closeShortcut)

    const { archiveThread, spamThread, trashThread, moveThread, toggleRead, toggleStar, updateLabel } =
        useThreadActions(threadStateCollection, threadState, navigateBack)

    const { threadIds } = useThreadListContext()
    const { hasPrevious, hasNext, goToPrevious, goToNext } = useThreadNavigation(threadIds, id)

    const { isScrolled, onScroll } = useScrollShadow()
    const [expandedMessages, setExpandedMessages] = useState<Set<string>>(new Set())

    const messageList = messages ?? []
    const lastMessage = messageList[messageList.length - 1] as MailMessages | undefined

    const isMessageExpanded = (msg: MailMessages, index: number) => {
        if (expandedMessages.has(msg.id)) return true
        return index === messageList.length - 1
    }

    const toggleExpanded = (msgId: string) => {
        setExpandedMessages((prev) => {
            const next = new Set(prev)
            if (next.has(msgId)) {
                next.delete(msgId)
            } else {
                next.add(msgId)
            }
            return next
        })
    }

    if (!threadState && !messages?.length) return <NotFoundState message="Email not found" />
    if (!hasAccess) return <NotFoundState message="Email not found" />

    const subject = messages?.[0]?.subject ?? ''

    const visibleAttachments = messageList.flatMap((msg, index) => {
        if (!isMessageExpanded(msg, index)) return []
        if (!msg.has_attachments || !msg.attachments?.length) return []
        return [{ recordId: msg.id, filenames: msg.attachments as string[] }]
    })

    const mailboxId = thread?.mailbox ?? ''

    const handleForwardAll = () => {
        if (!lastMessage) return
        openReply({
            messageId: lastMessage.id,
            threadId: id,
            subject: `Fwd: ${subject}`,
            to: [],
            mailboxId,
            sentToAddresses: [
                ...(lastMessage.recipients_to ?? []).map((r) => r.email),
                ...(lastMessage.recipients_cc ?? []).map((r) => r.email),
            ],
        })
    }

    return (
        <View className="flex-1" style={{ backgroundColor }}>
            <ScreenHeader isScrolled={isScrolled}>
                <EmailDetailToolbar
                    threadState={threadState}
                    labels={allLabels}
                    threadLabelIds={threadLabelIds}
                    onBack={navigateBack}
                    onArchive={() => archiveThread.mutate()}
                    onSpam={() => spamThread.mutate()}
                    onTrash={() => trashThread.mutate()}
                    onMove={(folder) => moveThread.mutate(folder)}
                    onUpdateLabel={(labelId, add) => updateLabel.mutate({ labelId, add })}
                    onToggleRead={() => toggleRead.mutate()}
                    onToggleStar={() => toggleStar.mutate()}
                    onForwardAll={handleForwardAll}
                    onNewer={goToPrevious}
                    onOlder={goToNext}
                    hasNewer={hasPrevious}
                    hasOlder={hasNext}
                />
            </ScreenHeader>
            <ScrollView
                className="flex-1"
                contentContainerStyle={{ flexGrow: 1 }}
                onScroll={onScroll}
                scrollEventThrottle={16}
            >
                <ThreadSubjectHeader subject={subject} labels={labels} />
                {messageList.map((msg, index) => {
                    const expanded = isMessageExpanded(msg, index)
                    return (
                        <View key={msg.id}>
                            <MessageHeader
                                senderName={msg.sender_name}
                                senderEmail={msg.sender_email}
                                date={msg.date}
                                isStarred={threadState?.is_starred}
                                deliveryStatus={msg.delivery_status}
                                bounceReason={msg.bounce_reason}
                                isExpanded={expanded}
                                onToggleExpand={() => toggleExpanded(msg.id)}
                                onToggleStar={() => toggleStar.mutate()}
                                onReply={() =>
                                    openReply({
                                        messageId: msg.id,
                                        threadId: id,
                                        subject: msg.subject,
                                        to: [{ name: msg.sender_name, email: msg.sender_email }],
                                        mailboxId,
                                        sentToAddresses: [
                                            ...(msg.recipients_to ?? []).map((r) => r.email),
                                            ...(msg.recipients_cc ?? []).map((r) => r.email),
                                        ],
                                    })
                                }
                                onReplyAll={() => {
                                    const replyAddresses = [
                                        ...(msg.recipients_to ?? []).map((r) => r.email),
                                        ...(msg.recipients_cc ?? []).map((r) => r.email),
                                    ]
                                    const defaultFrom = pickDefaultFrom({
                                        mode: 'reply',
                                        identities,
                                        replyToAddresses: replyAddresses,
                                    })
                                    const identity = identities.find((i) => i.mailboxId === defaultFrom.mailboxId)
                                    const rawTo = [
                                        { name: msg.sender_name, email: msg.sender_email },
                                        ...(msg.recipients_to ?? []),
                                        ...(msg.recipients_cc ?? []),
                                    ]
                                    const filteredTo = identity
                                        ? filterOwnAddresses({ identity, recipients: rawTo })
                                        : rawTo
                                    openReply({
                                        messageId: msg.id,
                                        threadId: id,
                                        subject: msg.subject,
                                        to: filteredTo,
                                        mailboxId,
                                        sentToAddresses: replyAddresses,
                                    })
                                }}
                                onForward={() =>
                                    openReply({
                                        messageId: msg.id,
                                        threadId: id,
                                        subject: `Fwd: ${msg.subject}`,
                                        to: [],
                                        mailboxId,
                                        sentToAddresses: [
                                            ...(msg.recipients_to ?? []).map((r) => r.email),
                                            ...(msg.recipients_cc ?? []).map((r) => r.email),
                                        ],
                                    })
                                }
                            />
                            {expanded ? (
                                <EmailBody collectionId="mail_messages" recordId={msg.id} filename={msg.body_html} />
                            ) : (
                                <CollapsedSnippet snippet={msg.snippet} onPress={() => toggleExpanded(msg.id)} />
                            )}
                        </View>
                    )
                })}
            </ScrollView>
            {visibleAttachments.map(({ recordId, filenames }) => (
                <EmailAttachments
                    key={recordId}
                    isVisible
                    collectionId="mail_messages"
                    recordId={recordId}
                    filenames={filenames}
                />
            ))}
            {lastMessage ? (
                <InlineReply
                    messageId={lastMessage.id}
                    threadId={id}
                    subject={lastMessage.subject}
                    senderName={lastMessage.sender_name}
                    senderEmail={lastMessage.sender_email}
                    recipientsTo={lastMessage.recipients_to ?? []}
                    recipientsCc={lastMessage.recipients_cc ?? []}
                    mailboxId={mailboxId}
                />
            ) : null}
        </View>
    )
}

function CollapsedSnippet({ snippet, onPress }: { snippet: string; onPress: () => void }) {
    const mutedColor = useThemeColor('muted-foreground')
    const borderColor = useThemeColor('border')
    return (
        <Pressable onPress={onPress}>
            <View
                className="px-4 py-2"
                style={{
                    borderBottomWidth: 1,
                    borderBottomColor: borderColor,
                }}
            >
                <Text style={{ fontSize: 13, color: mutedColor }} numberOfLines={1}>
                    {snippet}
                </Text>
            </View>
        </Pressable>
    )
}
