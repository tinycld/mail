import { usePathname } from 'one'
import { useEffect } from 'react'
import { AuthGate } from '~/components/workspace/AuthGate'
import { SkeletonLayout } from '~/components/workspace/SkeletonLayout'
import { useWorkspaceLayout } from '~/components/workspace/useWorkspaceLayout'
import { WorkspaceLayout } from '~/components/workspace/WorkspaceLayout'
import { WorkspaceLayoutProvider } from '~/components/workspace/WorkspaceLayoutProvider'
import { useAuth } from '~/lib/auth'

export default function OrgLayout() {
    return (
        <WorkspaceLayoutProvider>
            <OrgLayoutInner />
        </WorkspaceLayoutProvider>
    )
}

function OrgLayoutInner() {
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
            <ActiveAddonSync />
            <WorkspaceLayout />
        </>
    )
}

function ActiveAddonSync() {
    const pathname = usePathname()
    const { setActiveAddonSlug, setDrawerOpen } = useWorkspaceLayout()

    useEffect(() => {
        setDrawerOpen(false)

        const prefix = '/app/'
        if (!pathname.startsWith(prefix)) {
            setActiveAddonSlug(null)
            return
        }
        const rest = pathname.slice(prefix.length)
        const slug = rest.split('/')[0] || null
        setActiveAddonSlug(slug)
    }, [pathname, setActiveAddonSlug, setDrawerOpen])

    return null
}
