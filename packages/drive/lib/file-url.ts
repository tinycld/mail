import { pb } from '~/lib/pocketbase'
import type { DriveItemView } from '../types'

const DRIVE_ITEMS_COLLECTION = 'drive_items'

export function getFileURL(item: DriveItemView) {
    if (!item.file) return ''
    return pb.files.getURL({ collectionId: DRIVE_ITEMS_COLLECTION, id: item.id }, item.file)
}

export function getThumbnailURL(item: DriveItemView, size?: string) {
    if (item.thumbnail) {
        const url = pb.files.getURL(
            { collectionId: DRIVE_ITEMS_COLLECTION, id: item.id },
            item.thumbnail
        )
        return size ? `${url}?thumb=${size}` : url
    }

    if (item.category === 'image' && item.file) {
        const url = pb.files.getURL(
            { collectionId: DRIVE_ITEMS_COLLECTION, id: item.id },
            item.file
        )
        return `${url}?thumb=${size ?? '480x360'}`
    }

    return ''
}
