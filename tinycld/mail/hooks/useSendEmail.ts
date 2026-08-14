import {
    MultipartFieldAttachments,
    MultipartFieldJSON,
    type SendEmailRequest,
    type SendEmailResponse,
} from '@tinycld/app-generated/mail-api'
import { throttleProgress, uploadFormDataWithProgress } from '@tinycld/core/file-viewer/upload-file'
import { captureException, errorToString } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { notify } from '@tinycld/core/lib/notify'
import { PB_SERVER_ADDR, pb } from '@tinycld/core/lib/pocketbase'
import { useState } from 'react'

// The JSON body is the generated server contract; attachments ride alongside
// as multipart file parts, so they are a client-side extension of it.
type SendEmailParams = SendEmailRequest & { attachments?: File[] }

interface UseSendEmailOptions {
    onSuccess?: () => void
    onError?: (message: string) => void
}

export function useSendEmail({ onSuccess, onError }: UseSendEmailOptions = {}) {
    // null when nothing is uploading, else a fraction in [0,1]. Only the
    // attachment path ever sets it — a bodyless send has no bytes to report.
    const [uploadProgress, setUploadProgress] = useState<number | null>(null)

    const mutation = useMutation({
        mutationFn: async (params: SendEmailParams): Promise<SendEmailResponse> => {
            const { attachments, ...jsonFields } = params
            if (attachments?.length) {
                const formData = new FormData()
                formData.append(MultipartFieldJSON, JSON.stringify(jsonFields))
                for (const file of attachments) {
                    formData.append(MultipartFieldAttachments, file, file.name)
                }
                // XHR rather than fetch, via core's shared uploader: fetch
                // cannot report upload progress, so a 10MB send used to sit
                // silent until it either finished or failed.
                setUploadProgress(0)
                try {
                    return (await uploadFormDataWithProgress({
                        url: `${PB_SERVER_ADDR}/api/mail/send`,
                        formData,
                        authToken: pb.authStore.token,
                        onProgress: throttleProgress((loaded, total) => {
                            setUploadProgress(total > 0 ? Math.min(1, loaded / total) : 0)
                        }),
                    })) as SendEmailResponse
                } finally {
                    setUploadProgress(null)
                }
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
        /** [0,1] while attachment bytes are in flight; null otherwise. */
        uploadProgress,
    }
}
