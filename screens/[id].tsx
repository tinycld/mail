import { and, eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useLocalSearchParams, useRouter } from 'expo-router'
import { useCallback, useRef, useState } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'
import { ScreenHeader } from '~/components/ScreenHeader'
import { mutation, useMutation } from '~/lib/mutations'
import { useStore } from '~/lib/pocketbase'
import { useThemeColor } from '~/lib/use-app-theme'
import { useCurrentRole } from '~/lib/use-current-role'
import { useScrollShadow } from '~/lib/use-scroll-shadow'
import { EmailAttachments } from '../components/EmailAttachments'
import { EmailBody } from '../components/EmailBody'
import { EmailDetailToolbar } from '../components/EmailDetailToolbar'
import { MessageHeader, ThreadSubjectHeader } from '../components/EmailHeader'
import { InlineReply } from '../components/InlineReply'
import { NotFoundState } from '../components/NotFoundState'
import { useCompose } from '../hooks/useComposeState'
import { useLabels, useThreadLabels } from '../hooks/useLabels'
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
            yield threadStateCollection.update(stateId, draft => {
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
    const { userOrgId } = useCurrentRole()
    const router = useRouter()
    const { openReply } = useCompose()
    const _mutedColor = useThemeColor('muted-foreground')
    const _borderColor = useThemeColor('border')
    const backgroundColor = useThemeColor('background')

    const [threadStateCollection, messagesCollection] = useStore(
        'mail_thread_state',
        'mail_messages'
    )

    const { data: threadStates } = useLiveQuery(
        query =>
            query
                .from({ mail_thread_state: threadStateCollection })
                .where(({ mail_thread_state }) =>
                    and(eq(mail_thread_state.thread, id), eq(mail_thread_state.user_org, userOrgId))
                ),
        [id, userOrgId]
    )

    const threadState = threadStates?.[0]

    const { data: messages } = useLiveQuery(
        query =>
            query
                .from({ mail_messages: messagesCollection })
                .where(({ mail_messages }) => eq(mail_messages.thread, id))
                .orderBy(({ mail_messages }) => mail_messages.date, 'asc'),
        [id]
    )

    const { labels: allLabels } = useLabels()
    const { labels, labelIds: threadLabelIds } = useThreadLabels(threadState?.id ?? '')

    useAutoMarkAsRead(threadStateCollection, threadState, id)

    const navigateBack = useCallback(() => router.back(), [router])

    const {
        archiveThread,
        spamThread,
        trashThread,
        moveThread,
        toggleRead,
        toggleStar,
        updateLabel,
    } = useThreadActions(threadStateCollection, threadState, navigateBack)

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
        setExpandedMessages(prev => {
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

    const subject = messages?.[0]?.subject ?? ''

    const visibleAttachments = messageList.flatMap((msg, index) => {
        if (!isMessageExpanded(msg, index)) return []
        if (!msg.has_attachments || !msg.attachments?.length) return []
        return [{ recordId: msg.id, filenames: msg.attachments as string[] }]
    })

    const handleForwardAll = () => {
        if (!lastMessage) return
        openReply({
            messageId: lastMessage.id,
            threadId: id,
            subject: `Fwd: ${subject}`,
            to: [],
        })
    }

    return (
        <View style={{ flex: 1, backgroundColor }}>
            <ScreenHeader isScrolled={isScrolled}>
                <EmailDetailToolbar
                    threadState={threadState}
                    labels={allLabels}
                    threadLabelIds={threadLabelIds}
                    onArchive={() => archiveThread.mutate()}
                    onSpam={() => spamThread.mutate()}
                    onTrash={() => trashThread.mutate()}
                    onMove={folder => moveThread.mutate(folder)}
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
                style={{ flex: 1 }}
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
                                    })
                                }
                                onReplyAll={() =>
                                    openReply({
                                        messageId: msg.id,
                                        threadId: id,
                                        subject: msg.subject,
                                        to: [
                                            { name: msg.sender_name, email: msg.sender_email },
                                            ...(msg.recipients_to ?? []),
                                            ...(msg.recipients_cc ?? []),
                                        ],
                                    })
                                }
                                onForward={() =>
                                    openReply({
                                        messageId: msg.id,
                                        threadId: id,
                                        subject: `Fwd: ${msg.subject}`,
                                        to: [],
                                    })
                                }
                            />
                            {expanded ? (
                                <EmailBody
                                    collectionId="mail_messages"
                                    recordId={msg.id}
                                    filename={msg.body_html}
                                />
                            ) : (
                                <CollapsedSnippet
                                    snippet={msg.snippet}
                                    onPress={() => toggleExpanded(msg.id)}
                                />
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
                style={{
                    paddingHorizontal: 16,
                    paddingVertical: 8,
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
