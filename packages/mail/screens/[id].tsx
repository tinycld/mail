import { and, eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useParams } from 'one'
import { useMemo, useRef, useState } from 'react'
import { Pressable, ScrollView, StyleSheet, Text, View } from 'react-native'
import { useTheme, YStack } from 'tamagui'
import { useMutation } from '~/lib/mutations'
import { useStore } from '~/lib/pocketbase'
import { useCurrentRole } from '~/lib/use-current-role'
import { EmailBody } from '../components/EmailBody'
import { EmailDetailToolbar } from '../components/EmailDetailToolbar'
import { MessageHeader, ThreadSubjectHeader } from '../components/EmailHeader'
import { InlineReply } from '../components/InlineReply'
import { NotFoundState } from '../components/NotFoundState'
import type { MailMessages } from '../types'

export default function MailDetailScreen() {
    const { id = '' } = useParams<{ id: string }>()
    const { userOrgId } = useCurrentRole()

    const [threadStateCollection, messagesCollection, labelsCollection] = useStore(
        'mail_thread_state',
        'mail_messages',
        'mail_labels'
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

    const { data: allLabels } = useLiveQuery(
        query => query.from({ mail_labels: labelsCollection }),
        []
    )

    const labelMap = useMemo(() => {
        const map = new Map<string, { id: string; name: string; color: string }>()
        for (const l of allLabels ?? []) {
            map.set(l.id, l)
        }
        return map
    }, [allLabels])

    const labels = useMemo(() => {
        if (!threadState?.labels) return []
        const stateLabels: string[] = threadState.labels
        return stateLabels
            .map((lid: string) => labelMap.get(lid))
            .filter((l): l is { id: string; name: string; color: string } => l != null)
    }, [threadState?.labels, labelMap])

    const markAsRead = useMutation({
        mutationFn: function* (stateId: string) {
            yield threadStateCollection.update(stateId, draft => {
                draft.is_read = true
            })
        },
    })

    const markAsReadRef = useRef<string | null>(null)
    if (threadState && !threadState.is_read && markAsReadRef.current !== id) {
        markAsReadRef.current = id
        markAsRead.mutate(threadState.id)
    }

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

    return (
        <YStack flex={1} backgroundColor="$background">
            <EmailDetailToolbar />
            <ScrollView style={{ flex: 1 }} contentContainerStyle={{ flexGrow: 1 }}>
                <ThreadSubjectHeader subject={subject} labels={labels} />
                {messageList.map((msg, index) => {
                    const expanded = isMessageExpanded(msg, index)
                    return (
                        <View key={msg.id}>
                            <MessageHeader
                                senderName={msg.sender_name}
                                senderEmail={msg.sender_email}
                                date={msg.date}
                                deliveryStatus={msg.delivery_status}
                                bounceReason={msg.bounce_reason}
                                isExpanded={expanded}
                                onToggleExpand={() => toggleExpanded(msg.id)}
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
        </YStack>
    )
}

function CollapsedSnippet({ snippet, onPress }: { snippet: string; onPress: () => void }) {
    const theme = useTheme()
    return (
        <Pressable onPress={onPress}>
            <View style={[collapsedStyles.container, { borderBottomColor: theme.borderColor.val }]}>
                <Text style={[collapsedStyles.text, { color: theme.color8.val }]} numberOfLines={1}>
                    {snippet}
                </Text>
            </View>
        </Pressable>
    )
}

const collapsedStyles = StyleSheet.create({
    container: {
        paddingHorizontal: 16,
        paddingVertical: 8,
        borderBottomWidth: 1,
    },
    text: {
        fontSize: 13,
    },
})
