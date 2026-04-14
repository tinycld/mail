import { Forward, Reply, ReplyAll } from 'lucide-react-native'
import { useRef } from 'react'
import { Platform, Pressable, Text, View } from 'react-native'
import { useBreakpoint } from '~/components/workspace/useBreakpoint'
import { captureException } from '~/lib/errors'
import { useThemeColor } from '~/lib/use-app-theme'
import { useForm, zodResolver } from '~/ui/form'
import { composeSchema, parseRecipients } from '../hooks/composeSchema'
import { useAttachments } from '../hooks/useAttachments'
import { useCompose } from '../hooks/useComposeState'
import { useDefaultMailbox } from '../hooks/useDefaultMailbox'
import { useEditorHandle, useMailEditor } from '../hooks/useMailEditor'
import { useSendEmail } from '../hooks/useSendEmail'
import { AttachmentRibbon } from './AttachmentRibbon'
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
    const mutedColor = useThemeColor('muted-foreground')
    const borderColor = useThemeColor('border')
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

function InlineComposeForm({
    replyContext,
    onClose,
}: {
    replyContext: NonNullable<ReturnType<typeof useCompose>['replyContext']>
    onClose: () => void
}) {
    const borderColor = useThemeColor('border')
    const backgroundColor = useThemeColor('background')
    const _dangerColor = useThemeColor('danger')
    const editorRef = useRef<RichTextEditorHandle>(null)
    const fileInputRef = useRef<HTMLInputElement>(null)
    const mailboxId = useDefaultMailbox()
    const editor = useMailEditor({ placeholder: 'Compose reply' })
    useEditorHandle(editor, editorRef)
    const { attachments, addFiles, removeFile, clearAll: clearAttachments } = useAttachments()

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
    } = useForm({
        resolver: zodResolver(composeSchema),
        mode: 'onChange',
        defaultValues: { to: toValue, cc: '', bcc: '', subject: subjectValue },
    })

    const { send, isPending } = useSendEmail({
        onSuccess: () => {
            editorRef.current?.clear()
            clearAttachments()
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
            attachments: attachments.map(a => a.file),
        })
    })

    const handleAttach = () => {
        fileInputRef.current?.click()
    }

    const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        const files = e.target.files
        if (!files?.length) return
        try {
            addFiles(Array.from(files))
        } catch (err) {
            captureException('Failed to add attachments', err)
        }
        e.target.value = ''
    }

    return (
        <View
            className="m-4 border rounded-lg"
            style={{
                minHeight: 200,
                borderColor,
                backgroundColor,
            }}
        >
            <ComposeFields control={control} errors={errors} />
            <View className="flex-1 p-3" style={{ minHeight: 120 }}>
                <RichTextEditor editor={editor} />
            </View>
            <AttachmentRibbon
                isVisible={attachments.length > 0}
                attachments={attachments}
                onRemove={removeFile}
            />
            {Platform.OS === 'web' && (
                <input
                    ref={fileInputRef}
                    type="file"
                    multiple
                    style={{ display: 'none' }}
                    onChange={handleFileChange}
                />
            )}
            <ComposeToolbar
                editor={editor}
                onDiscard={onClose}
                onSend={onSend}
                onAttach={handleAttach}
                isPending={isPending}
            />
        </View>
    )
}
