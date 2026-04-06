import type { DriveItem, FolderTreeNode } from './types'

export const mockItems: DriveItem[] = [
    {
        id: 'f-projects',
        name: 'Projects',
        type: 'folder',
        mimeType: 'application/vnd.folder',
        owner: 'me',
        modifiedDate: '2026-04-02T10:30:00Z',
        size: 0,
        parentId: null,
        shared: false,
        starred: true,
    },
    {
        id: 'f-shared-docs',
        name: 'Shared Documents',
        type: 'folder',
        mimeType: 'application/vnd.folder',
        owner: 'me',
        modifiedDate: '2026-03-28T14:00:00Z',
        size: 0,
        parentId: null,
        shared: true,
        starred: false,
    },
    {
        id: 'f-personal',
        name: 'Personal',
        type: 'folder',
        mimeType: 'application/vnd.folder',
        owner: 'me',
        modifiedDate: '2026-03-15T09:00:00Z',
        size: 0,
        parentId: null,
        shared: false,
        starred: false,
    },
    {
        id: 'f-archive',
        name: 'Archive',
        type: 'folder',
        mimeType: 'application/vnd.folder',
        owner: 'me',
        modifiedDate: '2026-01-10T08:00:00Z',
        size: 0,
        parentId: null,
        shared: false,
        starred: false,
    },

    {
        id: 'f-q1-planning',
        name: 'Q1 Planning',
        type: 'folder',
        mimeType: 'application/vnd.folder',
        owner: 'me',
        modifiedDate: '2026-03-30T16:00:00Z',
        size: 0,
        parentId: 'f-projects',
        shared: true,
        starred: false,
    },
    {
        id: 'f-marketing',
        name: 'Marketing',
        type: 'folder',
        mimeType: 'application/vnd.folder',
        owner: 'me',
        modifiedDate: '2026-04-01T11:00:00Z',
        size: 0,
        parentId: 'f-projects',
        shared: false,
        starred: false,
    },
    {
        id: 'f-engineering',
        name: 'Engineering',
        type: 'folder',
        mimeType: 'application/vnd.folder',
        owner: 'me',
        modifiedDate: '2026-04-03T09:45:00Z',
        size: 0,
        parentId: 'f-projects',
        shared: true,
        starred: true,
    },

    {
        id: 'd-roadmap',
        name: 'Product Roadmap 2026',
        type: 'document',
        mimeType: 'application/vnd.google-apps.document',
        owner: 'me',
        modifiedDate: '2026-03-29T15:30:00Z',
        size: 245000,
        parentId: 'f-q1-planning',
        shared: true,
        starred: true,
    },
    {
        id: 's-budget',
        name: 'Q1 Budget',
        type: 'spreadsheet',
        mimeType: 'application/vnd.google-apps.spreadsheet',
        owner: 'Sarah Chen',
        modifiedDate: '2026-03-25T10:00:00Z',
        size: 512000,
        parentId: 'f-q1-planning',
        shared: true,
        starred: false,
    },
    {
        id: 'p-strategy',
        name: 'Strategy Deck',
        type: 'presentation',
        mimeType: 'application/vnd.google-apps.presentation',
        owner: 'me',
        modifiedDate: '2026-03-20T14:00:00Z',
        size: 3200000,
        parentId: 'f-q1-planning',
        shared: true,
        starred: false,
    },

    {
        id: 'i-brand-guide',
        name: 'Brand Guidelines',
        type: 'pdf',
        mimeType: 'application/pdf',
        owner: 'Alex Rivera',
        modifiedDate: '2026-03-18T11:30:00Z',
        size: 8500000,
        parentId: 'f-marketing',
        shared: true,
        starred: false,
    },
    {
        id: 'i-logo',
        name: 'Logo Variants',
        type: 'image',
        mimeType: 'image/png',
        owner: 'me',
        modifiedDate: '2026-03-15T09:30:00Z',
        size: 2400000,
        parentId: 'f-marketing',
        shared: false,
        starred: false,
    },
    {
        id: 'd-social-plan',
        name: 'Social Media Plan',
        type: 'document',
        mimeType: 'application/vnd.google-apps.document',
        owner: 'Jordan Kim',
        modifiedDate: '2026-04-01T16:20:00Z',
        size: 180000,
        parentId: 'f-marketing',
        shared: true,
        starred: false,
    },

    {
        id: 'f-api-docs',
        name: 'API Documentation',
        type: 'folder',
        mimeType: 'application/vnd.folder',
        owner: 'me',
        modifiedDate: '2026-04-03T09:45:00Z',
        size: 0,
        parentId: 'f-engineering',
        shared: true,
        starred: false,
    },
    {
        id: 'd-arch-overview',
        name: 'Architecture Overview',
        type: 'document',
        mimeType: 'application/vnd.google-apps.document',
        owner: 'me',
        modifiedDate: '2026-04-02T17:00:00Z',
        size: 320000,
        parentId: 'f-engineering',
        shared: true,
        starred: false,
    },
    {
        id: 'dr-system-diagram',
        name: 'System Diagram',
        type: 'drawing',
        mimeType: 'application/vnd.google-apps.drawing',
        owner: 'me',
        modifiedDate: '2026-03-27T13:00:00Z',
        size: 150000,
        parentId: 'f-engineering',
        shared: false,
        starred: true,
    },

    {
        id: 'd-api-v2',
        name: 'API v2 Reference',
        type: 'document',
        mimeType: 'application/vnd.google-apps.document',
        owner: 'me',
        modifiedDate: '2026-04-03T09:45:00Z',
        size: 420000,
        parentId: 'f-api-docs',
        shared: true,
        starred: false,
    },
    {
        id: 's-api-metrics',
        name: 'API Usage Metrics',
        type: 'spreadsheet',
        mimeType: 'application/vnd.google-apps.spreadsheet',
        owner: 'Taylor Park',
        modifiedDate: '2026-04-01T08:00:00Z',
        size: 890000,
        parentId: 'f-api-docs',
        shared: true,
        starred: false,
    },

    {
        id: 'd-meeting-notes',
        name: 'Team Meeting Notes',
        type: 'document',
        mimeType: 'application/vnd.google-apps.document',
        owner: 'Sarah Chen',
        modifiedDate: '2026-04-04T10:00:00Z',
        size: 95000,
        parentId: 'f-shared-docs',
        shared: true,
        starred: false,
    },
    {
        id: 's-team-roster',
        name: 'Team Roster',
        type: 'spreadsheet',
        mimeType: 'application/vnd.google-apps.spreadsheet',
        owner: 'me',
        modifiedDate: '2026-03-10T15:00:00Z',
        size: 128000,
        parentId: 'f-shared-docs',
        shared: true,
        starred: false,
    },
    {
        id: 'p-onboarding',
        name: 'Onboarding Slides',
        type: 'presentation',
        mimeType: 'application/vnd.google-apps.presentation',
        owner: 'Jordan Kim',
        modifiedDate: '2026-02-20T12:00:00Z',
        size: 5600000,
        parentId: 'f-shared-docs',
        shared: true,
        starred: true,
    },

    {
        id: 'd-resume',
        name: 'Resume 2026',
        type: 'document',
        mimeType: 'application/vnd.google-apps.document',
        owner: 'me',
        modifiedDate: '2026-03-01T10:00:00Z',
        size: 65000,
        parentId: 'f-personal',
        shared: false,
        starred: false,
    },
    {
        id: 'i-headshot',
        name: 'Profile Photo',
        type: 'image',
        mimeType: 'image/jpeg',
        owner: 'me',
        modifiedDate: '2026-02-15T14:00:00Z',
        size: 1800000,
        parentId: 'f-personal',
        shared: false,
        starred: false,
    },
    {
        id: 'pdf-tax',
        name: 'Tax Documents 2025',
        type: 'pdf',
        mimeType: 'application/pdf',
        owner: 'me',
        modifiedDate: '2026-01-20T09:00:00Z',
        size: 4200000,
        parentId: 'f-personal',
        shared: false,
        starred: false,
    },

    {
        id: 'd-old-proposal',
        name: 'Client Proposal (Old)',
        type: 'document',
        mimeType: 'application/vnd.google-apps.document',
        owner: 'me',
        modifiedDate: '2025-11-15T10:00:00Z',
        size: 210000,
        parentId: 'f-archive',
        shared: false,
        starred: false,
    },
    {
        id: 's-2025-financials',
        name: '2025 Financials',
        type: 'spreadsheet',
        mimeType: 'application/vnd.google-apps.spreadsheet',
        owner: 'Sarah Chen',
        modifiedDate: '2025-12-31T17:00:00Z',
        size: 1100000,
        parentId: 'f-archive',
        shared: true,
        starred: false,
    },
]

export function getChildren(parentId: string | null): DriveItem[] {
    return mockItems.filter(item => item.parentId === parentId)
}

export function getItem(id: string): DriveItem | undefined {
    return mockItems.find(item => item.id === id)
}

export function getBreadcrumbs(folderId: string | null): DriveItem[] {
    const crumbs: DriveItem[] = []
    let currentId = folderId
    while (currentId) {
        const item = getItem(currentId)
        if (!item) break
        crumbs.unshift(item)
        currentId = item.parentId
    }
    return crumbs
}

export function getFolderTree(): FolderTreeNode[] {
    const folders = mockItems.filter(item => item.type === 'folder')

    function buildTree(parentId: string | null): FolderTreeNode[] {
        return folders
            .filter(f => f.parentId === parentId)
            .map(folder => ({
                item: folder,
                children: buildTree(folder.id),
            }))
    }

    return buildTree(null)
}

export function getStarredItems(): DriveItem[] {
    return mockItems.filter(item => item.starred)
}

export function getRecentItems(): DriveItem[] {
    return [...mockItems]
        .filter(item => item.type !== 'folder')
        .sort((a, b) => new Date(b.modifiedDate).getTime() - new Date(a.modifiedDate).getTime())
        .slice(0, 10)
}

export function getSharedItems(): DriveItem[] {
    return mockItems.filter(item => item.shared && item.owner !== 'me')
}
