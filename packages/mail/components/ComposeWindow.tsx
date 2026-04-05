import { useEffect, useRef } from 'react'
import { Platform, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import { useBreakpoint } from '~/components/workspace/useBreakpoint'
import { performMutations } from '~/lib/mutations'
import { useStore } from '~/lib/pocketbase'
import { useForm, zodResolver } from '~/ui/form'
import { type ComposeFormData, composeSchema, parseRecipients } from '../hooks/composeSchema'
import { useCompose } from '../hooks/useComposeState'
import { useDefaultMailbox } from '../hooks/useDefaultMailbox'
import { useEditorHandle, useMailEditor } from '../hooks/useMailEditor'
import { useSaveDraft } from '../hooks/useSaveDraft'
import { useSendEmail } from '../hooks/useSendEmail'
import { ComposeFields } from './ComposeFields'
import { ComposeHeader } from './ComposeHeader'
import { ComposeToolbar } from './ComposeToolbar'
import type { RichTextEditorHandle } from './RichTextEditor'
import { RichTextEditor } from './RichTextEditor'

export type { ComposeFormData } from '../hooks/composeSchema'

const webShadow =
    Platform.OS === 'web'
        ? ({ boxShadow: '0 8px 32px rgba(0,0,0,0.24)' } as Record<string, unknown>)
        : {}

interface ComposeWindowProps {
    isVisible: boolean
}

export function ComposeWindow({ isVisible }: ComposeWindowProps) {
    const theme = useTheme()
    const { mode, replyContext, draftContext, minimize, maximize, open, close } = useCompose()
    const breakpoint = useBreakpoint()
    const editorRef = useRef<RichTextEditorHandle>(null)
    const mailboxId = useDefaultMailbox()
    const prevModeRef = useRef(mode)
    const draftIdRef = useRef<string | null>(null)

    const editor = useMailEditor({ placeholder: 'Compose email' })
    useEditorHandle(editor, editorRef)

    const {
        control,
        handleSubmit,
        reset,
        setError,
        getValues,
        formState: { errors },
    } = useForm<ComposeFormData>({
        resolver: zodResolver(composeSchema),
        mode: 'onChange',
        defaultValues: { to: '', cc: '', bcc: '', subject: '' },
    })

    useEffect(() => {
        const wasClosedOrNew = prevModeRef.current === 'closed'
        prevModeRef.current = mode

        if (draftContext) {
            draftIdRef.current = draftContext.messageId
            const formatRecipients = (recipients: { name: string; email: string }[]) =>
                recipients.map(r => (r.name ? `${r.name} <${r.email}>` : r.email)).join(', ')
            reset({
                to: formatRecipients(draftContext.to),
                cc: formatRecipients(draftContext.cc),
                bcc: formatRecipients(draftContext.bcc),
                subject: draftContext.subject,
            })
            editor.setContent(draftContext.htmlBody || draftContext.textBody || '')
        } else if (replyContext) {
            draftIdRef.current = null
            const toValue =
                replyContext.to.map(r => (r.name ? `${r.name} <${r.email}>` : r.email)).join(', ') +
                (replyContext.to.length > 0 ? ', ' : '')
            const subjectPrefix = replyContext.subject.startsWith('Re:')
                ? replyContext.subject
                : `Re: ${replyContext.subject}`
            reset({ to: toValue, cc: '', bcc: '', subject: subjectPrefix })
        } else if (mode === 'open' && wasClosedOrNew) {
            draftIdRef.current = null
            reset({ to: '', cc: '', bcc: '', subject: '' })
        }
    }, [replyContext, draftContext, mode, reset, editor])

    const [messagesCollection] = useStore('mail_messages')

    const deleteDraftMessage = async () => {
        const id = draftIdRef.current
        if (!id) return
        draftIdRef.current = null
        await performMutations(function* () {
            yield messagesCollection.delete(id)
        })
    }

    const { send, isPending } = useSendEmail({
        onSuccess: async () => {
            await deleteDraftMessage()
            editorRef.current?.clear()
            reset({ to: '', cc: '', bcc: '', subject: '' })
            close()
        },
    })

    const { saveDraft } = useSaveDraft()

    const handleClose = async () => {
        const text = await editorRef.current?.getText()
        if (!text?.trim() || !mailboxId) {
            close()
            return
        }

        const data = getValues()
        const htmlBody = (await editorRef.current?.getHTML()) ?? ''
        const to = data.to ? parseRecipients(data.to) : undefined
        const cc = data.cc ? parseRecipients(data.cc) : undefined
        const bcc = data.bcc ? parseRecipients(data.bcc) : undefined

        saveDraft({
            mailbox_id: mailboxId,
            message_id: draftIdRef.current ?? undefined,
            to,
            cc,
            bcc,
            subject: data.subject,
            html_body: htmlBody,
            text_body: text,
        })

        draftIdRef.current = null
        editorRef.current?.clear()
        reset({ to: '', cc: '', bcc: '', subject: '' })
        close()
    }

    if (!isVisible) return null

    const isMinimized = mode === 'minimized'
    const isMaximized = mode === 'maximized'
    const isNotDesktop = breakpoint !== 'desktop'
    const hasMailbox = mailboxId != null

    const modeStyleMap = {
        open: styles.normal,
        minimized: styles.minimized,
        maximized: styles.maximized,
        closed: styles.normal,
        inline: styles.normal,
    }
    const windowStyle = isNotDesktop ? styles.fullscreen : modeStyleMap[mode]

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
            in_reply_to_message_id: replyContext?.messageId,
        })
    })

    const composeWindow = (
        <View
            style={[
                styles.container,
                windowStyle,
                {
                    backgroundColor: theme.background.val,
                    borderColor: theme.borderColor.val,
                    ...webShadow,
                },
            ]}
        >
            <ComposeHeader
                mode={mode}
                onMinimize={isMinimized ? open : minimize}
                onMaximize={isMaximized ? open : maximize}
                onClose={handleClose}
            />
            {isMinimized ? null : (
                <>
                    <ComposeFields control={control} errors={errors} />
                    {hasMailbox ? null : (
                        <View style={styles.mailboxWarning}>
                            <Text style={[styles.mailboxWarningText, { color: theme.red10.val }]}>
                                No mailbox found. Ask your admin to add you to a mailbox.
                            </Text>
                        </View>
                    )}
                    <View style={styles.body}>
                        <RichTextEditor editor={editor} />
                    </View>
                    <ComposeToolbar
                        editor={editor}
                        onDiscard={close}
                        onSend={onSend}
                        isPending={isPending}
                    />
                </>
            )}
        </View>
    )

    if (isMaximized && !isNotDesktop) {
        return <View style={styles.backdrop}>{composeWindow}</View>
    }

    return composeWindow
}

const styles = StyleSheet.create({
    container: {
        position: 'absolute',
        borderWidth: 1,
        borderRadius: 8,
        zIndex: 1000,
    },
    normal: {
        bottom: 0,
        right: 16,
        width: 500,
        height: 560,
    },
    minimized: {
        bottom: 0,
        right: 16,
        width: 300,
        height: 40,
    },
    maximized: {
        position: 'relative',
        width: '75%',
        maxWidth: 900,
        height: '85%',
        maxHeight: 800,
    },
    backdrop: {
        position: 'absolute',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        backgroundColor: 'rgba(0,0,0,0.3)',
        zIndex: 1000,
        alignItems: 'center',
        justifyContent: 'center',
    },
    fullscreen: {
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
    },
    body: {
        flex: 1,
        padding: 12,
    },
    mailboxWarning: {
        paddingHorizontal: 12,
        paddingVertical: 6,
    },
    mailboxWarningText: {
        fontSize: 12,
    },
})
