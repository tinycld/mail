import {
    MultipartFieldAttachments,
    MultipartFieldJSON,
    type SendEmailRequest,
    type SendEmailResponse,
} from '@tinycld/app-generated/mail-api'
import { captureException, errorToString } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { notify } from '@tinycld/core/lib/notify'
import { PB_SERVER_ADDR, pb } from '@tinycld/core/lib/pocketbase'

// The JSON body is the generated server contract; attachments ride alongside
// as multipart file parts, so they are a client-side extension of it.
type SendEmailParams = SendEmailRequest & { attachments?: File[] }

interface UseSendEmailOptions {
    onSuccess?: () => void
    onError?: (message: string) => void
}

export function useSendEmail({ onSuccess, onError }: UseSendEmailOptions = {}) {
    const mutation = useMutation({
        mutationFn: async (params: SendEmailParams): Promise<SendEmailResponse> => {
            const { attachments, ...jsonFields } = params
            if (attachments?.length) {
                const formData = new FormData()
                formData.append(MultipartFieldJSON, JSON.stringify(jsonFields))
                for (const file of attachments) {
                    formData.append(MultipartFieldAttachments, file, file.name)
                }
                const res = await fetch(`${PB_SERVER_ADDR}/api/mail/send`, {
                    method: 'POST',
                    headers: { Authorization: pb.authStore.token },
                    body: formData,
                })
                if (!res.ok) {
                    const data = await res.json().catch(() => ({}))
                    throw new Error(data.message || `Send failed: ${res.status}`)
                }
                return await res.json()
            }

            return await pb.send('/api/mail/send', {
                method: 'POST',
                body: jsonFields,
            })
        },
        onSuccess,
        onError: (error: unknown) => {
            const message = errorToString(error)
            captureException('mail send failed', error)
            notify.emit({
                event: 'mail.send_failed',
                title: 'Send failed',
                body: message,
                durationMs: 8000,
                data: { error: message },
            })
            onError?.(message)
        },
    })

    return {
        send: mutation.mutate,
        isPending: mutation.isPending,
        error: mutation.error,
    }
}
