import { Slot, useActiveParams, usePathname } from 'one'
import { useEffect } from 'react'
import { Platform } from 'react-native'
import { AuthGate } from '~/components/workspace/AuthGate'
import { SkeletonLayout } from '~/components/workspace/SkeletonLayout'
import { useWorkspaceLayout } from '~/components/workspace/useWorkspaceLayout'
import { WorkspaceLayout } from '~/components/workspace/WorkspaceLayout'
import { WorkspaceLayoutProvider } from '~/components/workspace/WorkspaceLayoutProvider'
import { useAuth } from '~/lib/auth'

export default function OrgLayout() {
    const { orgSlug = '' } = useActiveParams<{ orgSlug: string }>()

    if (Platform.OS !== 'web') {
        return <Slot />
    }

    return (
        <WorkspaceLayoutProvider>
            <OrgLayoutInner orgSlug={orgSlug} />
        </WorkspaceLayoutProvider>
    )
}

function OrgLayoutInner({ orgSlug }: { orgSlug: string }) {
    const auth = useAuth({ throwIfAnon: false })

    if (auth.isInitializing) {
        return <SkeletonLayout />
    }

    if (!auth.isLoggedIn) {
        return (
            <>
                <SkeletonLayout />
                <AuthGate />
            </>
        )
    }

    return (
        <>
            <ActiveAddonSync orgSlug={orgSlug} />
            <WorkspaceLayout orgSlug={orgSlug} />
        </>
    )
}

function ActiveAddonSync({ orgSlug }: { orgSlug: string }) {
    const pathname = usePathname()
    const { setActiveAddonSlug } = useWorkspaceLayout()

    useEffect(() => {
        const prefix = `/app/${orgSlug}/`
        if (!pathname.startsWith(prefix)) {
            setActiveAddonSlug(null)
            return
        }
        const rest = pathname.slice(prefix.length)
        const slug = rest.split('/')[0] || null
        setActiveAddonSlug(slug)
    }, [pathname, orgSlug, setActiveAddonSlug])

    return null
}
