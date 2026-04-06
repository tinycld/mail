export type DriveItemType =
    | 'folder'
    | 'document'
    | 'spreadsheet'
    | 'pdf'
    | 'image'
    | 'drawing'
    | 'presentation'

export interface DriveItem {
    id: string
    name: string
    type: DriveItemType
    mimeType: string
    owner: string
    modifiedDate: string
    size: number
    parentId: string | null
    shared: boolean
    starred: boolean
}

export type ViewMode = 'list' | 'grid'

export type SidebarSection =
    | 'my-drive'
    | 'shared-drives'
    | 'shared-with-me'
    | 'recent'
    | 'starred'
    | 'trash'

export interface FolderTreeNode {
    item: DriveItem
    children: FolderTreeNode[]
}
