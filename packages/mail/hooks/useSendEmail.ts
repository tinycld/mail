import { errorToString } from '~/lib/errors'
import { useMutation } from '~/lib/mutations'
import { pb } from '~/lib/pocketbase'

interface SendEmailParams {
    mailbox_id: string
    to: { name: string; email: string }[]
    cc?: { name: string; email: string }[]
    bcc?: { name: string; email: string }[]
    subject: string
    html_body: string
    text_body: string
    in_reply_to_message_id?: string
}

interface UseSendEmailOptions {
    onSuccess?: () => void
    onError?: (message: string) => void
}

export function useSendEmail({ onSuccess, onError }: UseSendEmailOptions = {}) {
    const mutation = useMutation({
        mutationFn: async (params: SendEmailParams) => {
            return await pb.send('/api/mail/send', {
                method: 'POST',
                body: params,
            })
        },
        onSuccess,
        onError: (error: unknown) => {
            onError?.(errorToString(error))
        },
    })

    return {
        send: mutation.mutate,
        isPending: mutation.isPending,
        error: mutation.error,
    }
}
