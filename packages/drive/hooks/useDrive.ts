import { createContext, useCallback, useContext, useMemo, useState } from 'react'
import {
    getBreadcrumbs,
    getChildren,
    getItem,
    getRecentItems,
    getSharedItems,
    getStarredItems,
} from '../mock-data'
import type { DriveItem, SidebarSection, ViewMode } from '../types'

interface DriveContextValue {
    currentFolderId: string | null
    activeSection: SidebarSection
    selectedItemId: string | null
    viewMode: ViewMode
    currentItems: DriveItem[]
    breadcrumbs: DriveItem[]
    selectedItem: DriveItem | undefined
    navigateToFolder: (folderId: string | null) => void
    navigateToSection: (section: SidebarSection) => void
    selectItem: (itemId: string | null) => void
    setViewMode: (mode: ViewMode) => void
    openItem: (item: DriveItem) => void
}

export const DriveContext = createContext<DriveContextValue | null>(null)

export function useDrive(): DriveContextValue {
    const ctx = useContext(DriveContext)
    if (!ctx) throw new Error('useDrive must be used within DriveProvider')
    return ctx
}

export function useDriveState(): DriveContextValue {
    const [currentFolderId, setCurrentFolderId] = useState<string | null>(null)
    const [activeSection, setActiveSection] = useState<SidebarSection>('my-drive')
    const [selectedItemId, setSelectedItemId] = useState<string | null>(null)
    const [viewMode, setViewMode] = useState<ViewMode>('list')

    const currentItems = useMemo(() => {
        switch (activeSection) {
            case 'starred':
                return getStarredItems()
            case 'recent':
                return getRecentItems()
            case 'shared-with-me':
                return getSharedItems()
            case 'my-drive':
                return getChildren(currentFolderId)
            default:
                return []
        }
    }, [activeSection, currentFolderId])

    const breadcrumbs = useMemo(() => getBreadcrumbs(currentFolderId), [currentFolderId])
    const selectedItem = useMemo(
        () => (selectedItemId ? getItem(selectedItemId) : undefined),
        [selectedItemId]
    )

    const navigateToFolder = useCallback((folderId: string | null) => {
        setCurrentFolderId(folderId)
        setActiveSection('my-drive')
        setSelectedItemId(null)
    }, [])

    const navigateToSection = useCallback((section: SidebarSection) => {
        setActiveSection(section)
        if (section !== 'my-drive') {
            setCurrentFolderId(null)
        }
        setSelectedItemId(null)
    }, [])

    const selectItem = useCallback((itemId: string | null) => {
        setSelectedItemId(itemId)
    }, [])

    const openItem = useCallback(
        (item: DriveItem) => {
            if (item.type === 'folder') {
                navigateToFolder(item.id)
            } else {
                setSelectedItemId(item.id)
            }
        },
        [navigateToFolder]
    )

    return useMemo(
        () => ({
            currentFolderId,
            activeSection,
            selectedItemId,
            viewMode,
            currentItems,
            breadcrumbs,
            selectedItem,
            navigateToFolder,
            navigateToSection,
            selectItem,
            setViewMode,
            openItem,
        }),
        [
            currentFolderId,
            activeSection,
            selectedItemId,
            viewMode,
            currentItems,
            breadcrumbs,
            selectedItem,
            navigateToFolder,
            navigateToSection,
            selectItem,
            openItem,
        ]
    )
}
