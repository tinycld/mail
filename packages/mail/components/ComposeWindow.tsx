import { useEffect, useRef } from 'react'
import { Platform, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import { useBreakpoint } from '~/components/workspace/useBreakpoint'
import { useForm, zodResolver } from '~/ui/form'
import { type ComposeFormData, composeSchema, parseRecipients } from '../hooks/composeSchema'
import { useCompose } from '../hooks/useComposeState'
import { useDefaultMailbox } from '../hooks/useDefaultMailbox'
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
    const { mode, replyContext, minimize, maximize, open, close } = useCompose()
    const breakpoint = useBreakpoint()
    const editorRef = useRef<RichTextEditorHandle>(null)
    const mailboxId = useDefaultMailbox()
    const prevModeRef = useRef(mode)

    const {
        control,
        handleSubmit,
        reset,
        setError,
        formState: { errors },
    } = useForm<ComposeFormData>({
        resolver: zodResolver(composeSchema),
        mode: 'onChange',
        defaultValues: { to: '', subject: '' },
    })

    useEffect(() => {
        const wasClosedOrNew = prevModeRef.current === 'closed'
        prevModeRef.current = mode

        if (replyContext) {
            const toValue = replyContext.to.map(r => r.email).join(', ')
            const subjectPrefix = replyContext.subject.startsWith('Re:')
                ? replyContext.subject
                : `Re: ${replyContext.subject}`
            reset({ to: toValue, subject: subjectPrefix })
        } else if (mode === 'open' && wasClosedOrNew) {
            reset({ to: '', subject: '' })
        }
    }, [replyContext, mode, reset])

    const { send, isPending } = useSendEmail({
        onSuccess: () => {
            editorRef.current?.clear()
            reset({ to: '', subject: '' })
            close()
        },
    })

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

        send({
            mailbox_id: mailboxId,
            to: parseRecipients(data.to),
            subject: data.subject,
            html_body: htmlBody,
            text_body: textBody,
            in_reply_to_message_id: replyContext?.messageId,
        })
    })

    return (
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
                onClose={close}
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
                        <RichTextEditor editorRef={editorRef} />
                    </View>
                    <ComposeToolbar onDiscard={close} onSend={onSend} isPending={isPending} />
                </>
            )}
        </View>
    )
}

const styles = StyleSheet.create({
    container: {
        position: 'absolute',
        borderWidth: 1,
        borderRadius: 8,
        overflow: 'hidden',
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
        bottom: 0,
        right: 16,
        width: 700,
        height: 700,
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
