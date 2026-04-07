import * as DocumentPicker from 'expo-document-picker'
import { newRecordId } from 'pbtsdb'
import { useCallback, useRef, useState } from 'react'
import { Platform } from 'react-native'
import { captureException } from '~/lib/errors'
import { performMutations, useMutation } from '~/lib/mutations'
import { pb, useStore } from '~/lib/pocketbase'

function deduplicateName(name: string, existingNames: Set<string>): string {
    if (!existingNames.has(name)) return name

    const dotIdx = name.lastIndexOf('.')
    const base = dotIdx > 0 ? name.slice(0, dotIdx) : name
    const ext = dotIdx > 0 ? name.slice(dotIdx) : ''

    for (let counter = 1; counter <= 999; counter++) {
        const candidate = `${base} (${counter})${ext}`
        if (!existingNames.has(candidate)) return candidate
    }
    return `${base} (${Date.now()})${ext}`
}

interface UploadingFile {
    name: string
    status: 'pending' | 'uploading' | 'done' | 'error'
}

interface UseFileUploadOptions {
    orgId: string
    userOrgId: string
    currentFolderId: string
}

export function useFileUpload({ orgId, userOrgId, currentFolderId }: UseFileUploadOptions) {
    const [uploadingFiles, setUploadingFiles] = useState<UploadingFile[]>([])
    const folderRef = useRef(currentFolderId)
    folderRef.current = currentFolderId
    const [sharesCollection] = useStore('drive_shares')

    const uploadMutation = useMutation({
        mutationFn: async (files: File[]) => {
            setUploadingFiles(files.map(f => ({ name: f.name, status: 'pending' })))

            const existing = await pb.collection('drive_items').getFullList({
                filter: pb.filter('org = {:org} && parent = {:parent}', {
                    org: orgId,
                    parent: folderRef.current || '',
                }),
                fields: 'name',
            })
            const usedNames = new Set(existing.map(r => r.name))

            for (let i = 0; i < files.length; i++) {
                const file = files[i]
                setUploadingFiles(prev =>
                    prev.map((f, idx) => (idx === i ? { ...f, status: 'uploading' } : f))
                )

                try {
                    const itemId = newRecordId()
                    const uniqueName = deduplicateName(file.name, usedNames)
                    usedNames.add(uniqueName)
                    const formData = new FormData()
                    formData.append('id', itemId)
                    formData.append('org', orgId)
                    formData.append('name', uniqueName)
                    formData.append('is_folder', 'false')
                    formData.append('mime_type', file.type || 'application/octet-stream')
                    formData.append('parent', folderRef.current)
                    formData.append('created_by', userOrgId)
                    formData.append('size', String(file.size))
                    formData.append('file', file)
                    formData.append('description', '')

                    // File upload requires FormData, so pb.collection() is necessary here
                    await pb.collection('drive_items').create(formData)

                    await performMutations(function* () {
                        yield sharesCollection.insert({
                            id: newRecordId(),
                            item: itemId,
                            user_org: userOrgId,
                            role: 'owner',
                            created_by: userOrgId,
                        })
                    })

                    setUploadingFiles(prev =>
                        prev.map((f, idx) => (idx === i ? { ...f, status: 'done' } : f))
                    )
                } catch (err) {
                    setUploadingFiles(prev =>
                        prev.map((f, idx) => (idx === i ? { ...f, status: 'error' } : f))
                    )
                    captureException('useFileUpload', err)
                    throw err
                }
            }
        },
        onSettled: () => {
            setTimeout(() => setUploadingFiles([]), 3000)
        },
    })

    const uploadFiles = useCallback(
        (files: File[]) => {
            if (files.length === 0) return
            uploadMutation.mutate(files)
        },
        [uploadMutation]
    )

    const triggerFilePicker = useCallback(() => {
        if (Platform.OS === 'web') {
            const input = document.createElement('input')
            input.type = 'file'
            input.multiple = true
            input.onchange = () => {
                if (input.files?.length) {
                    uploadFiles(Array.from(input.files))
                }
            }
            input.click()
        } else {
            DocumentPicker.getDocumentAsync({ multiple: true }).then(result => {
                if (result.canceled) return
                const files = result.assets.map(
                    asset =>
                        ({
                            uri: asset.uri,
                            name: asset.name,
                            type: asset.mimeType || 'application/octet-stream',
                            size: asset.size ?? 0,
                        }) as unknown as File
                )
                uploadFiles(files)
            })
        }
    }, [uploadFiles])

    const uploadNewVersion = useCallback(async (itemId: string, file: File) => {
        const formData = new FormData()
        formData.append('item', itemId)
        formData.append('file', file)
        await pb.send('/api/drive/upload-version', {
            method: 'POST',
            body: formData,
        })
    }, [])

    return {
        uploadFiles,
        isUploading: uploadMutation.isPending,
        uploadingFiles,
        triggerFilePicker,
        uploadNewVersion,
    }
}
