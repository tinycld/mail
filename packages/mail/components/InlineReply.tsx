import { Forward, Reply, ReplyAll } from 'lucide-react-native'
import { useRef } from 'react'
import { Pressable, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import { useBreakpoint } from '~/components/workspace/useBreakpoint'
import { useForm, zodResolver } from '~/ui/form'
import { type ComposeFormData, composeSchema, parseRecipients } from '../hooks/composeSchema'
import { useCompose } from '../hooks/useComposeState'
import { useDefaultMailbox } from '../hooks/useDefaultMailbox'
import { useSendEmail } from '../hooks/useSendEmail'
import { ComposeFields } from './ComposeFields'
import { ComposeToolbar } from './ComposeToolbar'
import type { RichTextEditorHandle } from './RichTextEditor'
import { RichTextEditor } from './RichTextEditor'

interface InlineReplyProps {
    messageId: string
    threadId: string
    subject: string
    senderName: string
    senderEmail: string
    recipientsTo: { name: string; email: string }[]
    recipientsCc: { name: string; email: string }[]
}

export function InlineReply({
    messageId,
    threadId,
    subject,
    senderName,
    senderEmail,
    recipientsTo,
    recipientsCc,
}: InlineReplyProps) {
    const theme = useTheme()
    const breakpoint = useBreakpoint()
    const isMobile = breakpoint === 'mobile'
    const { mode, replyContext, openReply, close } = useCompose()

    const isInlineActive = mode === 'inline' && replyContext?.threadId === threadId

    const handleReply = () => {
        openReply({
            messageId,
            threadId,
            subject,
            to: [{ name: senderName, email: senderEmail }],
        })
    }

    const handleReplyAll = () => {
        const allRecipients = [
            { name: senderName, email: senderEmail },
            ...recipientsTo,
            ...recipientsCc,
        ]
        openReply({
            messageId,
            threadId,
            subject,
            to: allRecipients,
        })
    }

    const handleForward = () => {
        openReply({
            messageId,
            threadId,
            subject: `Fwd: ${subject}`,
            to: [],
        })
    }

    if (isInlineActive) {
        const formKey = `${replyContext.messageId}-${replyContext.to.length}`
        return <InlineComposeForm key={formKey} replyContext={replyContext} onClose={close} />
    }

    return (
        <View
            style={[
                styles.container,
                isMobile && styles.containerMobile,
                { borderTopColor: theme.borderColor.val },
            ]}
        >
            <Pressable
                style={[styles.actionButton, { borderColor: theme.borderColor.val }]}
                onPress={handleReply}
            >
                <Reply size={16} color={theme.color8.val} />
                <Text style={[styles.actionText, { color: theme.color8.val }]}>Reply</Text>
            </Pressable>
            <Pressable
                style={[styles.actionButton, { borderColor: theme.borderColor.val }]}
                onPress={handleReplyAll}
            >
                <ReplyAll size={16} color={theme.color8.val} />
                <Text style={[styles.actionText, { color: theme.color8.val }]}>Reply all</Text>
            </Pressable>
            <Pressable
                style={[styles.actionButton, { borderColor: theme.borderColor.val }]}
                onPress={handleForward}
            >
                <Forward size={16} color={theme.color8.val} />
                <Text style={[styles.actionText, { color: theme.color8.val }]}>Forward</Text>
            </Pressable>
        </View>
    )
}

function InlineComposeForm({
    replyContext,
    onClose,
}: {
    replyContext: NonNullable<ReturnType<typeof useCompose>['replyContext']>
    onClose: () => void
}) {
    const theme = useTheme()
    const editorRef = useRef<RichTextEditorHandle>(null)
    const mailboxId = useDefaultMailbox()

    const toValue =
        replyContext.to.map(r => (r.name ? `${r.name} <${r.email}>` : r.email)).join(', ') +
        (replyContext.to.length > 0 ? ', ' : '')
    const subjectValue = replyContext.subject.startsWith('Re:')
        ? replyContext.subject
        : `Re: ${replyContext.subject}`

    const {
        control,
        handleSubmit,
        reset,
        setError,
        formState: { errors },
    } = useForm<ComposeFormData>({
        resolver: zodResolver(composeSchema),
        mode: 'onChange',
        defaultValues: { to: toValue, cc: '', bcc: '', subject: subjectValue },
    })

    const { send, isPending } = useSendEmail({
        onSuccess: () => {
            editorRef.current?.clear()
            reset({ to: '', cc: '', bcc: '', subject: '' })
            onClose()
        },
    })

    const onSend = handleSubmit(async data => {
        if (!mailboxId) {
            setError('to', { message: 'No mailbox configured — contact your admin' })
            return
        }

        const htmlBody = (await editorRef.current?.getHTML()) ?? ''
        const textBody = (await editorRef.current?.getText()) ?? ''

        const cc = data.cc ? parseRecipients(data.cc) : undefined
        const bcc = data.bcc ? parseRecipients(data.bcc) : undefined

        send({
            mailbox_id: mailboxId,
            to: parseRecipients(data.to),
            cc,
            bcc,
            subject: data.subject,
            html_body: htmlBody,
            text_body: textBody,
            in_reply_to_message_id: replyContext.messageId,
        })
    })

    return (
        <View
            style={[
                inlineStyles.container,
                {
                    borderColor: theme.borderColor.val,
                    backgroundColor: theme.background.val,
                },
            ]}
        >
            <ComposeFields control={control} errors={errors} />
            <View style={inlineStyles.body}>
                <RichTextEditor editorRef={editorRef} />
            </View>
            <ComposeToolbar onDiscard={onClose} onSend={onSend} isPending={isPending} />
        </View>
    )
}

const styles = StyleSheet.create({
    container: {
        flexDirection: 'row',
        gap: 8,
        padding: 16,
        borderTopWidth: 1,
    },
    containerMobile: {
        flexWrap: 'wrap',
    },
    actionButton: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 6,
        paddingHorizontal: 16,
        paddingVertical: 8,
        borderRadius: 20,
        borderWidth: 1,
    },
    actionText: {
        fontSize: 13,
        fontWeight: '500',
    },
})

const inlineStyles = StyleSheet.create({
    container: {
        margin: 16,
        borderWidth: 1,
        borderRadius: 8,
        minHeight: 200,
    },
    body: {
        flex: 1,
        padding: 12,
        minHeight: 120,
    },
})
