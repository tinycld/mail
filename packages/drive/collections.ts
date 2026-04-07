import type { createCollection } from 'pbtsdb'
import { BasicIndex } from 'pbtsdb'
import type { CoreStores } from '~/lib/pocketbase'
import type { Schema } from '~/types/pbSchema'
import type { DriveSchema } from './types'

type MergedSchema = Schema & DriveSchema

export function registerCollections(
    newCollection: ReturnType<typeof createCollection<MergedSchema>>,
    coreStores: CoreStores
) {
    const drive_items = newCollection('drive_items', {
        omitOnInsert: ['created', 'updated', 'thumbnail'] as const,
        expand: { created_by: coreStores.user_org },
        collectionOptions: {
            autoIndex: 'eager' as const,
            defaultIndexType: BasicIndex,
        },
    })

    const drive_shares = newCollection('drive_shares', {
        omitOnInsert: ['created', 'updated'] as const,
        expand: {
            item: drive_items,
            user_org: coreStores.user_org,
            created_by: coreStores.user_org,
        },
        collectionOptions: {
            autoIndex: 'eager' as const,
            defaultIndexType: BasicIndex,
        },
    })

    const drive_item_state = newCollection('drive_item_state', {
        omitOnInsert: ['created', 'updated'] as const,
        expand: { item: drive_items, user_org: coreStores.user_org },
        collectionOptions: {
            autoIndex: 'eager' as const,
            defaultIndexType: BasicIndex,
        },
    })

    const drive_item_versions = newCollection('drive_item_versions', {
        omitOnInsert: ['created', 'updated'] as const,
        expand: { item: drive_items, created_by: coreStores.user_org },
        collectionOptions: {
            autoIndex: 'eager' as const,
            defaultIndexType: BasicIndex,
        },
    })

    return {
        drive_items,
        drive_shares,
        drive_item_state,
        drive_item_versions,
    }
}
