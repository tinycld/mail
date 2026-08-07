import {
    MultipartFieldAttachments,
    MultipartFieldJSON,
    type SaveDraftRequest,
    type SendEmailResponse,
} from '@tinycld/app-generated/mail-api'
import { captureException, errorToString } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { PB_SERVER_ADDR, pb } from '@tinycld/core/lib/pocketbase'

// The JSON body is the generated server contract; attachments ride alongside
// as multipart file parts, so they are a client-side extension of it.
type SaveDraftParams = SaveDraftRequest & { attachments?: File[] }

interface UseSaveDraftOptions {
    onSuccess?: () => void
    onError?: (message: string) => void
}

export function useSaveDraft({ onSuccess, onError }: UseSaveDraftOptions = {}) {
    const mutation = useMutation({
        mutationFn: async (params: SaveDraftParams): Promise<SendEmailResponse> => {
            const { attachments, ...jsonFields } = params

            if (attachments?.length) {
                const formData = new FormData()
                formData.append(MultipartFieldJSON, JSON.stringify(jsonFields))
                for (const file of attachments) {
                    formData.append(MultipartFieldAttachments, file, file.name)
                }
                const res = await fetch(`${PB_SERVER_ADDR}/api/mail/draft`, {
                    method: 'POST',
                    headers: { Authorization: pb.authStore.token },
                    body: formData,
                })
                if (!res.ok) {
                    const data = await res.json().catch(() => ({}))
                    throw new Error(data.message || `Draft save failed: ${res.status}`)
                }
                return await res.json()
            }

            return await pb.send('/api/mail/draft', {
                method: 'POST',
                body: jsonFields,
            })
        },
        onSuccess,
        onError: (error: unknown) => {
            // Same contract as the useSendEmail sibling: a lost draft is real
            // data loss and must reach Sentry, not just the caller's toast.
            captureException('mail.draft.save', error)
            onError?.(errorToString(error))
        },
    })

    return {
        saveDraft: mutation.mutate,
        isPending: mutation.isPending,
        error: mutation.error,
    }
}
