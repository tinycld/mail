import type { createCollection } from 'pbtsdb'
import { BasicIndex } from 'pbtsdb'
import type { Schema } from '~/types/pbSchema'
import type { CoreStores } from '~/lib/pocketbase'
import type { ContactsSchema } from './types'

type MergedSchema = Schema & ContactsSchema

export function registerCollections(
    newCollection: ReturnType<typeof createCollection<MergedSchema>>,
    coreStores: CoreStores,
) {
    const contacts = newCollection('contacts', {
        omitOnInsert: ['created', 'updated'] as const,
        expand: { owner: coreStores.user_org },
        collectionOptions: {
            autoIndex: 'eager' as const,
            defaultIndexType: BasicIndex,
        },
    })
    return { contacts }
}
