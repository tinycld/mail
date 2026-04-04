import AsyncStorage from '@react-native-async-storage/async-storage'
import { createContext, type ReactNode, useCallback, useContext, useEffect, useState } from 'react'
import {
    authStoreReady,
    clearStores,
    fetchAndSeedUserOrg,
    getUserFromAuthStore,
    pb,
    preloadStores,
    seedUserOrg,
} from '~/lib/pocketbase'
import type { UserSession } from '~/lib/types'
import type { Orgs, UserOrg, Users } from '~/types/pbSchema'

interface UserOrgExpanded extends UserOrg {
    expand?: { org?: Orgs }
}

export class AuthRequiredError extends Error {
    constructor(message = 'Authentication required') {
        super(message)
        this.name = 'AuthRequiredError'
    }
}

type AuthenticatedUser = UserSession

type LoginResult = {
    user: AuthenticatedUser | null
    error: string | null
}

type AuthActions = {
    login: (email: string, password: string) => Promise<LoginResult>
    logout: () => void
}

type AuthenticatedContext = AuthActions & {
    isLoggedIn: true
    user: AuthenticatedUser
}

type AuthContextType =
    | (AuthActions & {
          isLoggedIn: true
          user: AuthenticatedUser
          isInitializing: boolean
      })
    | (AuthActions & {
          isLoggedIn: false
          user: null
          isInitializing: boolean
      })

const AuthContext = createContext<AuthContextType | undefined>(undefined)

interface AuthProviderProps {
    children: ReactNode
}

const PRIMARY_ORG_STORAGE_KEY = 'tinycld_primary_org'

async function savePrimaryOrgToStorage(orgSlug: string): Promise<void> {
    try {
        await AsyncStorage.setItem(PRIMARY_ORG_STORAGE_KEY, orgSlug)
    } catch {
        // Storage might not be available
    }
}

export async function loadPrimaryOrgFromStorage(): Promise<string | null> {
    try {
        return await AsyncStorage.getItem(PRIMARY_ORG_STORAGE_KEY)
    } catch {
        return null
    }
}

async function clearPrimaryOrgStorage(): Promise<void> {
    try {
        await AsyncStorage.removeItem(PRIMARY_ORG_STORAGE_KEY)
    } catch {
        // Storage might not be available
    }
}

export function AuthProvider({ children }: AuthProviderProps) {
    const [hasHydrated, setHasHydrated] = useState(false)
    const [user, setUser] = useState<AuthenticatedUser | null>(null)

    const refreshUser = useCallback(async () => {
        const primaryOrgSlug = await loadPrimaryOrgFromStorage()
        const currentUser = getUserFromAuthStore(primaryOrgSlug)
        if (!currentUser) {
            setUser(null)
            await clearPrimaryOrgStorage()
            return
        }
        setUser(currentUser)
    }, [])

    useEffect(() => {
        const initAuth = async () => {
            await authStoreReady

            const primaryOrgSlug = await loadPrimaryOrgFromStorage()
            const currentUser = getUserFromAuthStore(primaryOrgSlug)

            if (currentUser) {
                setUser(currentUser)
                await fetchAndSeedUserOrg()
                await preloadStores()
            }
            setHasHydrated(true)
        }

        initAuth()

        const unsubscribe = pb.authStore.onChange(() => {
            refreshUser()
        })
        return unsubscribe
    }, [refreshUser])

    const login = async (email: string, password: string): Promise<LoginResult> => {
        pb.authStore.clear()
        try {
            const authData = await pb.collection('users').authWithPassword<
                Users & {
                    expand?: {
                        user_org_via_user?: UserOrgExpanded[]
                    }
                }
            >(email, password, {
                expand: 'user_org_via_user.org',
            })
            const userOrgs = authData.record.expand?.user_org_via_user ?? []
            const firstUserOrgWithSlug = userOrgs.find(uo => uo.expand?.org?.slug)

            if (!firstUserOrgWithSlug?.expand?.org) {
                pb.authStore.clear()
                return {
                    user: null,
                    error: 'No organization associated with this account',
                }
            }

            const primaryOrgSlug = firstUserOrgWithSlug.expand.org.slug

            const authenticatedUser: AuthenticatedUser = {
                id: authData.record.id,
                name: authData.record.name,
                email: authData.record.email,
                primaryOrgSlug,
            }

            const { expand: _, ...userOrgRecord } = firstUserOrgWithSlug
            await seedUserOrg(authData.record, firstUserOrgWithSlug.expand.org, userOrgRecord)

            await savePrimaryOrgToStorage(primaryOrgSlug)
            setUser(authenticatedUser)
            await preloadStores()

            return { user: authenticatedUser, error: null }
        } catch (error) {
            const message = error instanceof Error ? error.message : 'Failed to sign in'
            return { user: null, error: message }
        }
    }

    const logout = () => {
        pb.authStore.clear()
        clearPrimaryOrgStorage()
        clearStores()
        setUser(null)
    }

    const isLoggedIn = !!user && !!pb.authStore.token

    const contextValue: AuthContextType = isLoggedIn
        ? {
              login,
              logout,
              user,
              isLoggedIn: true,
              isInitializing: !hasHydrated,
          }
        : {
              login,
              logout,
              user: null,
              isLoggedIn: false,
              isInitializing: !hasHydrated,
          }

    return <AuthContext.Provider value={contextValue}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthenticatedContext
export function useAuth(options: { throwIfAnon: true }): AuthenticatedContext
export function useAuth(options: { throwIfAnon: false }): AuthContextType
export function useAuth(options?: {
    throwIfAnon: boolean
}): AuthenticatedContext | AuthContextType {
    const context = useContext(AuthContext)
    if (context === undefined) {
        throw new Error('useAuth must be used within an AuthProvider')
    }

    const throwIfAnon = options?.throwIfAnon ?? true
    if (throwIfAnon && !context.isLoggedIn && !context.isInitializing) {
        throw new AuthRequiredError('User must be authenticated to access this resource')
    }

    if (throwIfAnon) {
        return context as AuthenticatedContext
    }

    return context
}
