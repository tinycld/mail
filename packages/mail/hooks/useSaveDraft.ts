import { errorToString } from '~/lib/errors'
import { useMutation } from '~/lib/mutations'
import { pb } from '~/lib/pocketbase'

interface SaveDraftParams {
    mailbox_id: string
    message_id?: string
    to?: { name: string; email: string }[]
    cc?: { name: string; email: string }[]
    bcc?: { name: string; email: string }[]
    subject?: string
    html_body: string
    text_body: string
}

interface UseSaveDraftOptions {
    onSuccess?: () => void
    onError?: (message: string) => void
}

export function useSaveDraft({ onSuccess, onError }: UseSaveDraftOptions = {}) {
    const mutation = useMutation({
        mutationFn: async (params: SaveDraftParams) => {
            return await pb.send('/api/mail/draft', {
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
        saveDraft: mutation.mutate,
        isPending: mutation.isPending,
        error: mutation.error,
    }
}
