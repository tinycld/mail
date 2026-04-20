import { useState } from 'react'
import { formatBytes } from '@tinycld/core/lib/format-utils'

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

    const totalSize = attachments.reduce((sum, a) => sum + a.size, 0)

    const addFiles = (files: File[]) => {
        const incoming = files.map((file) => ({
            id: `${file.name}-${file.size}-${Date.now()}-${Math.random()}`,
            name: file.name,
            size: file.size,
            type: file.type,
            file,
        }))

        setAttachments((prev) => {
            const combined = [...prev, ...incoming]
            if (combined.length > MAX_FILES) {
                throw new Error(`Maximum ${MAX_FILES} attachments allowed`)
            }
            const newTotal = combined.reduce((sum, a) => sum + a.size, 0)
            if (newTotal > MAX_TOTAL_SIZE) {
                throw new Error(`Total attachment size exceeds ${formatBytes(MAX_TOTAL_SIZE)}`)
            }
            return combined
        })
    }

    const removeFile = (id: string) => {
        setAttachments((prev) => prev.filter((a) => a.id !== id))
    }

    const clearAll = () => {
        setAttachments([])
    }

    return { attachments, totalSize, addFiles, removeFile, clearAll }
}
