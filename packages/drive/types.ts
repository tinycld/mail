import type { Orgs, UserOrg } from '~/types/pbSchema'

export interface DriveItems {
    id: string
    org: string
    name: string
    is_folder: boolean
    mime_type: string
    parent: string
    created_by: string
    size: number
    file: string
    thumbnail: string
    description: string
    created: string
    updated: string
}

export interface DriveShares {
    id: string
    item: string
    user_org: string
    role: 'owner' | 'editor' | 'viewer'
    created_by: string
    created: string
    updated: string
}

export interface DriveItemState {
    id: string
    item: string
    user_org: string
    is_starred: boolean
    trashed_at: string
    last_viewed_at: string
    created: string
    updated: string
}

export type FileCategory =
    | 'folder'
    | 'document'
    | 'spreadsheet'
    | 'pdf'
    | 'image'
    | 'presentation'
    | 'drawing'
    | 'video'
    | 'audio'
    | 'archive'
    | 'code'
    | 'unknown'

export interface DriveItemView {
    id: string
    name: string
    isFolder: boolean
    mimeType: string
    parentId: string
    owner: string
    ownerUserOrgId: string
    updated: string
    size: number
    shared: boolean
    starred: boolean
    trashedAt: string
    file: string
    thumbnail: string
    description: string
    category: FileCategory
}

export interface FolderTreeNode {
    item: DriveItemView
    children: FolderTreeNode[]
}

export type ViewMode = 'list' | 'grid'

export type SidebarSection = 'my-drive' | 'shared-with-me' | 'recent' | 'starred' | 'trash'

export interface DriveItemVersions {
    id: string
    item: string
    version_number: number
    file: string
    size: number
    mime_type: string
    source: 'upload' | 'system'
    label: string
    created_by: string
    created: string
    updated: string
}

export type DriveSchema = {
    drive_items: {
        type: DriveItems
        relations: {
            org: Orgs
            parent: DriveItems
            created_by: UserOrg
        }
    }
    drive_shares: {
        type: DriveShares
        relations: {
            item: DriveItems
            user_org: UserOrg
            created_by: UserOrg
        }
    }
    drive_item_state: {
        type: DriveItemState
        relations: {
            item: DriveItems
            user_org: UserOrg
        }
    }
    drive_item_versions: {
        type: DriveItemVersions
        relations: {
            item: DriveItems
            created_by: UserOrg
        }
    }
}
