import { useBreakpoint } from '@tinycld/core/components/workspace/useBreakpoint'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Forward, Reply, ReplyAll } from 'lucide-react-native'
import { lazy, Suspense } from 'react'
import { Pressable, Text, View } from 'react-native'
import { buildReplyContext, type ReplyKind } from '../hooks/replyContext'
import { useCompose } from '../hooks/useComposeState'
import { useOpenReply } from '../hooks/useOpenReply'
import { useSendableIdentities } from '../hooks/useSendableIdentities'

// Lazy boundary keeps the rich-text editor (and its transitive
// prosemirror-view top-level DOM access) out of the static import
// graph that mounts at app launch via expo-router's <Stack>.
const InlineComposeForm = lazy(() => import('./InlineComposeForm'))

interface InlineReplyProps {
    messageId: string
    threadId: string
    subject: string
    senderName: string
    senderEmail: string
    recipientsTo: { name: string; email: string }[]
    recipientsCc: { name: string; email: string }[]
    mailboxId: string
    deliveredTo: string
}

export function InlineReply({
    messageId,
    threadId,
    subject,
    senderName,
    senderEmail,
    recipientsTo,
    recipientsCc,
    mailboxId,
    deliveredTo,
}: InlineReplyProps) {
    const mutedColor = useThemeColor('muted-foreground')
    const borderColor = useThemeColor('border')
    const breakpoint = useBreakpoint()
    const isMobile = breakpoint === 'mobile'
    const { mode, replyContext, close } = useCompose()
    const openReply = useOpenReply()
    const identities = useSendableIdentities()

    const isInlineActive = mode === 'inline' && replyContext?.threadId === threadId

    const message = {
        id: messageId,
        subject,
        sender_name: senderName,
        sender_email: senderEmail,
        recipients_to: recipientsTo,
        recipients_cc: recipientsCc,
        delivered_to: deliveredTo,
    }
    const start = (kind: ReplyKind) =>
        openReply(
            buildReplyContext({ kind, message, threadId, threadMailboxId: mailboxId, identities })
        )
    const handleReply = () => start('reply')
    const handleReplyAll = () => start('replyAll')
    const handleForward = () => start('forward')

    if (isInlineActive) {
        const formKey = `${replyContext.messageId}-${replyContext.to.length}`
        return (
            <Suspense fallback={null}>
                <InlineComposeForm key={formKey} replyContext={replyContext} onClose={close} />
            </Suspense>
        )
    }

    return (
        <View
            className="flex-row gap-2 p-4"
            style={{
                borderTopWidth: 1,
                borderTopColor: borderColor,
                flexWrap: isMobile ? 'wrap' : undefined,
            }}
        >
            <Pressable
                className="flex-row items-center px-4 py-2 rounded-full border"
                style={{
                    gap: 6,
                    borderColor,
                }}
                onPress={handleReply}
            >
                <Reply size={16} color={mutedColor} />
                <Text style={{ fontSize: 13, fontWeight: '500', color: mutedColor }}>Reply</Text>
            </Pressable>
            <Pressable
                className="flex-row items-center px-4 py-2 rounded-full border"
                style={{
                    gap: 6,
                    borderColor,
                }}
                onPress={handleReplyAll}
            >
                <ReplyAll size={16} color={mutedColor} />
                <Text style={{ fontSize: 13, fontWeight: '500', color: mutedColor }}>
                    Reply all
                </Text>
            </Pressable>
            <Pressable
                className="flex-row items-center px-4 py-2 rounded-full border"
                style={{
                    gap: 6,
                    borderColor,
                }}
                onPress={handleForward}
            >
                <Forward size={16} color={mutedColor} />
                <Text style={{ fontSize: 13, fontWeight: '500', color: mutedColor }}>Forward</Text>
            </Pressable>
        </View>
    )
}
