import type { ComponentType } from 'react'
import type { DriveItemView } from '../types'

export interface ThumbnailProps {
    item: DriveItemView
    size?: number
}

export interface PreviewProps {
    item: DriveItemView
    onClose: () => void
    onNext?: () => void
    onPrevious?: () => void
}

interface PreviewRegistryEntry {
    thumbnail?: ComponentType<ThumbnailProps>
    preview: ComponentType<PreviewProps>
}

const registry = new Map<string, PreviewRegistryEntry>()

export function registerPreview(pattern: string, entry: PreviewRegistryEntry) {
    registry.set(pattern, entry)
}

export function getPreviewEntry(mimeType: string): PreviewRegistryEntry | undefined {
    const exact = registry.get(mimeType)
    if (exact) return exact

    const wildcard = `${mimeType.split('/')[0]}/*`
    const wildcardEntry = registry.get(wildcard)
    if (wildcardEntry) return wildcardEntry

    return registry.get('*')
}
