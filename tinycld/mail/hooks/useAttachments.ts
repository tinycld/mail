import { captureException } from '@tinycld/core/lib/errors'
import { formatBytes } from '@tinycld/core/lib/format-utils'
import { notify } from '@tinycld/core/lib/notify'
import { useCallback, useRef, useState } from 'react'

// Thrown for the expected too-many/too-big cases so addFilesSafely can tell a
// validation refusal (toast only — not a bug) from a genuine failure inside
// addFiles (toast AND capture).
class AttachmentValidationError extends Error {}

export interface AttachmentFile {
    id: string
    name: string
    size: number
    type: string
    file: File
}

const MAX_TOTAL_SIZE = 10 * 1024 * 1024 // 10MB (Postmark limit)
const MAX_FILES = 20

export function useAttachments() {
    const [attachments, setAttachments] = useState<AttachmentFile[]>([])
    // React batches state updates and runs the updater asynchronously, so a throw inside
    // setAttachments(prev => ...) escapes any try/catch around addFiles. Validate first
    // against a ref-mirrored current value so addFiles can throw synchronously.
    const attachmentsRef = useRef<AttachmentFile[]>([])
    attachmentsRef.current = attachments

    const totalSize = attachments.reduce((sum, a) => sum + a.size, 0)

    const addFiles = useCallback((files: File[]) => {
        const incoming = files.map(file => ({
            id: `${file.name}-${file.size}-${Date.now()}-${Math.random()}`,
            name: file.name,
            size: file.size,
            type: file.type,
            file,
        }))

        const combined = [...attachmentsRef.current, ...incoming]
        if (combined.length > MAX_FILES) {
            throw new AttachmentValidationError(`Maximum ${MAX_FILES} attachments allowed`)
        }
        const newTotal = combined.reduce((sum, a) => sum + a.size, 0)
        if (newTotal > MAX_TOTAL_SIZE) {
            throw new AttachmentValidationError(
                `Total attachment size exceeds ${formatBytes(MAX_TOTAL_SIZE)}`
            )
        }

        setAttachments(combined)
    }, [])

    // Wraps addFiles with the standard "show a toast on validation failure" handling
    // so callers (paperclip picker, drag-drop) don't each need to reimplement it.
    const addFilesSafely = useCallback(
        (files: File[]) => {
            try {
                addFiles(files)
            } catch (err) {
                // Validation refusals are user feedback, not bugs; anything
                // else in here IS a bug and must not hide behind the toast.
                if (!(err instanceof AttachmentValidationError)) {
                    captureException('mail.attachments.add', err)
                }
                const reason = err instanceof Error ? err.message : 'Could not attach files'
                notify.emit({
                    event: 'mail.attachments_rejected',
                    title: 'Could not attach files',
                    body: reason,
                    durationMs: 6000,
                    data: { reason },
                })
            }
        },
        [addFiles]
    )

    const removeFile = useCallback((id: string) => {
        setAttachments(prev => prev.filter(a => a.id !== id))
    }, [])

    const clearAll = useCallback(() => {
        setAttachments([])
    }, [])

    return { attachments, totalSize, addFiles, addFilesSafely, removeFile, clearAll }
}
