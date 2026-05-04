import { useBreakpoint } from '@tinycld/core/components/workspace/useBreakpoint'
import { usePickFiles } from '@tinycld/core/file-viewer/use-pick-files'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useForm, zodResolver } from '@tinycld/core/ui/form'
import { Forward, Reply, ReplyAll } from 'lucide-react-native'
import { useCallback } from 'react'
import { KeyboardAvoidingView, Platform, Pressable, Text, View } from 'react-native'
import { composeSchema, parseRecipients } from '../hooks/composeSchema'
import { filterOwnAddresses, pickDefaultFrom } from '../hooks/defaultFromIdentity'
import { useAttachments } from '../hooks/useAttachments'
import { useCompose } from '../hooks/useComposeState'
import { useDefaultMailbox } from '../hooks/useDefaultMailbox'
import { useFileDrop } from '../hooks/useFileDrop'
import { useMailEditor } from '../hooks/useMailEditor'
import { useSendableIdentities } from '../hooks/useSendableIdentities'
import { useSendEmail } from '../hooks/useSendEmail'
import { useComposeStore } from '../stores/compose-store'
import { AttachmentRibbon } from './AttachmentRibbon'
import { ComposeFields } from './ComposeFields'
import { ComposeToolbar } from './ComposeToolbar'
import { DropOverlay } from './DropOverlay'

interface InlineReplyProps {
    messageId: string
    threadId: string
    subject: string
    senderName: string
    senderEmail: string
    recipientsTo: { name: string; email: string }[]
    recipientsCc: { name: string; email: string }[]
    mailboxId: string
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
}: InlineReplyProps) {
    const mutedColor = useThemeColor('muted-foreground')
    const borderColor = useThemeColor('border')
    const breakpoint = useBreakpoint()
    const isMobile = breakpoint === 'mobile'
    const { mode, replyContext, openReply, close } = useCompose()
    const identities = useSendableIdentities()

    const isInlineActive = mode === 'inline' && replyContext?.threadId === threadId

    const sentToAddresses = [...recipientsTo.map((r) => r.email), ...recipientsCc.map((r) => r.email)]

    const handleReply = () => {
        openReply({
            messageId,
            threadId,
            subject,
            to: [{ name: senderName, email: senderEmail }],
            mailboxId,
            sentToAddresses,
        })
    }

    const handleReplyAll = () => {
        const defaultFrom = pickDefaultFrom({
            mode: 'reply',
            identities,
            replyToAddresses: sentToAddresses,
        })
        const identity = identities.find((i) => i.mailboxId === defaultFrom.mailboxId)
        const rawTo = [{ name: senderName, email: senderEmail }, ...recipientsTo, ...recipientsCc]
        const filteredTo = identity ? filterOwnAddresses({ identity, recipients: rawTo }) : rawTo
        openReply({
            messageId,
            threadId,
            subject,
            to: filteredTo,
            mailboxId,
            sentToAddresses,
        })
    }

    const handleForward = () => {
        openReply({
            messageId,
            threadId,
            subject: `Fwd: ${subject}`,
            to: [],
            mailboxId,
            sentToAddresses,
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
                <Text style={{ fontSize: 13, fontWeight: '500', color: mutedColor }}>Reply all</Text>
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
    const mailboxId = useDefaultMailbox()
    const aliasId = useComposeStore((s) => s.aliasId)
    const { editor, EditorComponent, commands, toolbarState } = useMailEditor({
        placeholder: 'Compose reply',
        autofocus: true,
    })
    const { attachments, addFilesSafely, removeFile, clearAll: clearAttachments } = useAttachments()

    const toValue =
        replyContext.to.map((r) => (r.name ? `${r.name} <${r.email}>` : r.email)).join(', ') +
        (replyContext.to.length > 0 ? ', ' : '')
    const subjectValue = replyContext.subject.startsWith('Re:') ? replyContext.subject : `Re: ${replyContext.subject}`

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
            editor.clear()
            clearAttachments()
            reset({ to: '', cc: '', bcc: '', subject: '' })
            onClose()
        },
    })

    const onSend = handleSubmit(async (data) => {
        if (!mailboxId) {
            setError('to', { message: 'No mailbox configured — contact your admin' })
            return
        }

        const htmlBody = await editor.getHTML()
        const textBody = await editor.getText()

        const cc = data.cc ? parseRecipients(data.cc) : undefined
        const bcc = data.bcc ? parseRecipients(data.bcc) : undefined

        send({
            mailbox_id: mailboxId,
            alias_id: aliasId ?? undefined,
            to: parseRecipients(data.to),
            cc,
            bcc,
            subject: data.subject,
            html_body: htmlBody,
            text_body: textBody,
            in_reply_to_message_id: replyContext.messageId,
            attachments: attachments.map((a) => a.file),
        })
    })

    const { isDragging, dropRef } = useFileDrop({
        onFiles: addFilesSafely,
        isEnabled: !!mailboxId,
    })

    const { pickFiles, ActionSheetElement: PickerActionSheet } = usePickFiles()
    const handleAttach = useCallback(async () => {
        const picked = await pickFiles({ sources: ['photoLibrary', 'camera', 'documents'], multiple: true })
        if (picked.length > 0) addFilesSafely(picked.map((p) => p.file))
    }, [pickFiles, addFilesSafely])

    return (
        <KeyboardAvoidingView
            behavior={Platform.OS === 'ios' ? 'padding' : undefined}
            className="m-4 border rounded-lg"
            style={{
                minHeight: 200,
                borderColor,
                backgroundColor,
            }}
        >
            <View ref={dropRef} className="flex-1">
                <ComposeFields control={control} errors={errors} />
                <View className="p-3" style={{ minHeight: 120, maxHeight: 280 }}>
                    <EditorComponent />
                </View>
                <AttachmentRibbon
                    isVisible={attachments.length > 0}
                    attachments={attachments}
                    onRemove={removeFile}
                />
                <ComposeToolbar
                    commands={commands}
                    toolbarState={toolbarState}
                    onDiscard={onClose}
                    onSend={onSend}
                    onAttach={handleAttach}
                    isPending={isPending}
                />
                <DropOverlay isVisible={isDragging} />
            </View>
            {PickerActionSheet}
        </KeyboardAvoidingView>
    )
}
