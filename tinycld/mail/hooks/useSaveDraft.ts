import {
    MultipartFieldAttachments,
    MultipartFieldJSON,
    type SaveDraftRequest,
    type SendEmailResponse,
} from '@tinycld/app-generated/mail-api'
import { uploadFormDataWithProgress } from '@tinycld/core/file-viewer/upload-file'
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
                // Shares the send path's XHR transport. No progress callback:
                // a draft save is a background autosave with no surface to
                // report into, so it only wants the one code path, not a bar.
                return (await uploadFormDataWithProgress({
                    url: `${PB_SERVER_ADDR}/api/mail/draft`,
                    formData,
                    authToken: pb.authStore.token,
                })) as SendEmailResponse
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
