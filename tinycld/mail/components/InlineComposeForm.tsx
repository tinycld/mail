import { usePickFiles } from '@tinycld/core/file-viewer/use-pick-files'
import { useFileDrop } from '@tinycld/core/lib/file-drop/use-file-drop'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useForm, zodResolver } from '@tinycld/core/ui/form'
import { useCallback, useEffect } from 'react'
import { View } from 'react-native'
import { composeSchema, parseRecipients } from '../hooks/composeSchema'
import { useComposeAttachments } from '../hooks/useComposeAttachments'
import { useComposeFrom } from '../hooks/useComposeFrom'
import type { useCompose } from '../hooks/useComposeState'
import { useMailEditor } from '../hooks/useMailEditor'
import { useSendEmail } from '../hooks/useSendEmail'
import { AttachmentRibbon } from './AttachmentRibbon'
import { ComposeFields } from './ComposeFields'
import { ComposeFromRow } from './ComposeFromRow'
import { ComposeToolbar } from './ComposeToolbar'
import { DropOverlay } from './DropOverlay'

interface InlineComposeFormProps {
    replyContext: NonNullable<ReturnType<typeof useCompose>['replyContext']>
    onClose: () => void
}

export default function InlineComposeForm({ replyContext, onClose }: InlineComposeFormProps) {
    const borderColor = useThemeColor('border')
    const backgroundColor = useThemeColor('background')
    // A reply goes from the mailbox the reply context chose (the mailbox of
    // the address the message arrived on), else the thread's mailbox.
    const { choices, from, setFrom } = useComposeFrom(replyContext.mailboxId)
    const { editor, EditorComponent, commands, toolbarState } = useMailEditor({
        placeholder: 'Compose reply',
    })
    // An inline reply never resumes a draft, so it has no saved files.
    const {
        items: attachmentItems,
        files: attachmentFiles,
        addFilesSafely,
        removeItem,
        clearAll: clearAttachments,
    } = useComposeAttachments(null)

    // Forward starts with an empty To, so focus the recipient field; reply /
    // reply-all pre-fill To, so focus the body instead — the user wants to type
    // the message, not edit recipients.
    const toIsEmpty = replyContext.to.length === 0

    useEffect(() => {
        if (!toIsEmpty) editor.focus('start')
    }, [editor, toIsEmpty])

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
        formState: { errors },
    } = useForm({
        resolver: zodResolver(composeSchema),
        mode: 'onChange',
        defaultValues: { to: toValue, cc: '', bcc: '', subject: subjectValue },
    })

    const { send, isPending, uploadProgress } = useSendEmail({
        onSuccess: () => {
            editor.clear()
            clearAttachments()
            reset({ to: '', cc: '', bcc: '', subject: '' })
            onClose()
        },
    })

    // Send is disabled without a resolved From (matching ComposeWindow); the
    // guard only narrows the type.
    const onSend = handleSubmit(async data => {
        if (!from) return

        const htmlBody = await editor.getHTML()
        const textBody = await editor.getText()

        const cc = data.cc ? parseRecipients(data.cc) : undefined
        const bcc = data.bcc ? parseRecipients(data.bcc) : undefined

        send({
            mailbox_id: from.identity.mailboxId,
            from_address_id: from.address.id,
            to: parseRecipients(data.to),
            cc,
            bcc,
            subject: data.subject,
            html_body: htmlBody,
            text_body: textBody,
            in_reply_to_message_id: replyContext.messageId,
            attachments: attachmentFiles,
        })
    })

    const { isDragging, dropRef } = useFileDrop({
        onFiles: addFilesSafely,
        isEnabled: !!from,
    })

    const { pickFiles } = usePickFiles()
    const handleAttach = useCallback(async () => {
        const picked = await pickFiles({
            sources: ['photoLibrary', 'camera', 'documents'],
            multiple: true,
        })
        if (picked.length > 0) addFilesSafely(picked.map(p => p.file))
    }, [pickFiles, addFilesSafely])

    return (
        <View
            className="m-4 border rounded-lg"
            style={{
                minHeight: 200,
                borderColor,
                backgroundColor,
            }}
        >
            <View ref={dropRef} className="flex-1">
                <ComposeFromRow identities={choices} from={from} onSelect={setFrom} />
                <ComposeFields control={control} errors={errors} autoFocusTo={toIsEmpty} />
                <View className="p-3" style={{ minHeight: 120, maxHeight: 280 }}>
                    <EditorComponent />
                </View>
                <AttachmentRibbon
                    isVisible={attachmentItems.length > 0}
                    items={attachmentItems}
                    onRemove={removeItem}
                    uploadProgress={uploadProgress}
                />
                <ComposeToolbar
                    commands={commands}
                    toolbarState={toolbarState}
                    onDiscard={onClose}
                    onSend={onSend}
                    onAttach={handleAttach}
                    isPending={isPending}
                    isSendDisabled={!from}
                />
                <DropOverlay isVisible={isDragging} />
            </View>
        </View>
    )
}
