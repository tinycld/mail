import AsyncStorage from '@react-native-async-storage/async-storage'
import { QueryClient } from '@tanstack/react-query'
import { BasicIndex, createCollection, createReactProvider, setLogger } from 'pbtsdb'
import PocketBase, { AsyncAuthStore } from 'pocketbase'
import { Platform } from 'react-native'
import { addonStores, type MergedSchema } from '~/lib/generated/addon-collections'
import type { Orgs, UserOrg, Users } from '~/types/pbSchema'
import { PB_SERVER_ADDR } from './config'
import type { UserSession } from './types'

export { eq } from '@tanstack/db'

if (Platform.OS !== 'web') {
    // Only polyfill EventSource on native — the browser has its own
    import('react-native-sse').then(mod => {
        global.EventSource = mod.default as unknown as typeof global.EventSource
    })
}

export { PB_SERVER_ADDR }

const initialAuthPromise =
    typeof window !== 'undefined' ? AsyncStorage.getItem('pb_auth') : Promise.resolve(null)

const store = new AsyncAuthStore({
    save: async serialized => AsyncStorage.setItem('pb_auth', serialized),
    initial: initialAuthPromise,
    clear: async () => await AsyncStorage.removeItem('pb_auth'),
})

export const authStoreReady = initialAuthPromise.then(async storedAuth => {
    if (storedAuth) {
        let attempts = 0
        while (!store.token && attempts < 10) {
            await new Promise(resolve => setTimeout(resolve, 10))
            attempts++
        }
    }
})

export const pb = new PocketBase(PB_SERVER_ADDR, store)

pb.autoCancellation(false)

export function usePocketBase() {
    return pb
}

setLogger({
    debug: () => {},
    info: (_msg, context) => {
        if (context) {
        } else {
        }
    },
    warn: (_msg, context) => {
        if (context) {
        } else {
        }
    },
    error: (_msg, context) => {
        if (context) {
        } else {
        }
    },
})

const queryClient = new QueryClient()

const newCollection = createCollection<MergedSchema>(pb, queryClient)

const indexing = {
    collectionOptions: {
        autoIndex: 'eager' as const,
        defaultIndexType: BasicIndex,
    },
}

const users = newCollection('users', {
    omitOnInsert: ['created', 'updated', 'password', 'tokenKey'],
    ...indexing,
})

const orgs = newCollection('orgs', {
    omitOnInsert: ['created', 'updated'],
    ...indexing,
})

const user_org = newCollection('user_org', {
    omitOnInsert: ['created', 'updated'],
    expand: {
        user: users,
        org: orgs,
    },
    ...indexing,
})

const settings = newCollection('settings', {
    omitOnInsert: ['created', 'updated'],
    expand: { org: orgs },
    ...indexing,
})

const coreStores = { users, orgs, user_org, settings }
export type CoreStores = typeof coreStores

const stores = {
    ...coreStores,
    ...addonStores(newCollection, coreStores),
}

const { Provider: PBTSDBProvider, useStore } = createReactProvider(stores)

export function getUserFromAuthStore(primaryOrgSlug?: string | null): UserSession | null {
    const authRecord = pb.authStore.record as Users | null
    const authToken = pb.authStore.token

    if (!authRecord || !authToken || !pb.authStore.isValid) {
        return null
    }

    return {
        id: authRecord.id,
        name: authRecord.name,
        email: authRecord.email,
        primaryOrgSlug: primaryOrgSlug ?? undefined,
    }
}

export async function seedUserOrg(userRecord: Users, orgRecord: Orgs, userOrgRecord: UserOrg) {
    await Promise.all([stores.users.preload(), stores.orgs.preload(), stores.user_org.preload()])
    stores.users.utils?.writeUpsert(userRecord)
    stores.orgs.utils?.writeUpsert(orgRecord)
    stores.user_org.utils?.writeUpsert(userOrgRecord)
}

export async function preloadStores() {
    await Promise.all([stores.orgs.preload(), stores.user_org.preload()])
}

export async function fetchAndSeedUserOrg() {
    await Promise.all([stores.users.preload(), stores.orgs.preload(), stores.user_org.preload()])
    const userOrgs = await pb.collection('user_org').getFullList<UserOrg>()
    for (const userOrgRecord of userOrgs) {
        stores.user_org.utils?.writeUpsert(userOrgRecord)
    }
}

export async function clearStores() {
    for (const s of Object.values(stores)) {
        await s.cleanup()
    }
}

export { PBTSDBProvider, queryClient, stores, useStore }
