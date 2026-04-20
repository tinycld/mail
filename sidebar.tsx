import { eq } from '@tanstack/db'
import { useGlobalSearchParams, usePathname, useRouter } from 'expo-router'
import { AlertTriangle, Archive, File, Inbox, Mail, Pencil, Send, Settings, Star, Trash2 } from 'lucide-react-native'
import { useMemo, useState } from 'react'
import { Pressable } from 'react-native'
import { LabelManagerDialog } from '~/components/LabelManagerDialog'
import {
    SidebarActionButton,
    SidebarDivider,
    SidebarHeading,
    SidebarItem,
    SidebarNav,
} from '~/components/sidebar-primitives'
import { useOrgHref } from '~/lib/org-routes'
import { useStore } from '~/lib/pocketbase'
import { useThemeColor } from '~/lib/use-app-theme'
import { useOrgLiveQuery } from '~/lib/use-org-live-query'
import { composeEvents } from './hooks/composeEvents'
import { useLabels } from './hooks/useLabels'

interface MailSidebarProps {
    isCollapsed: boolean
}

function useActiveView() {
    const pathname = usePathname()
    const { folder, label } = useGlobalSearchParams<{ folder?: string; label?: string }>()
    if (pathname.includes('/mail/')) return { folder: null, activeLabels: new Set<string>() }
    const activeLabels = new Set(label ? label.split(',').filter(Boolean) : [])
    return { folder: activeLabels.size > 0 ? null : (folder ?? 'inbox'), activeLabels }
}

export default function MailSidebar(_props: MailSidebarProps) {
    const router = useRouter()
    const mutedColor = useThemeColor('muted-foreground')
    const { folder: activeFolder, activeLabels } = useActiveView()
    const orgHref = useOrgHref()
    const [labelManagerOpen, setLabelManagerOpen] = useState(false)

    const [threadStateCollection] = useStore('mail_thread_state')
    const { labels: orgLabels } = useLabels()

    const { data: threadStates } = useOrgLiveQuery((query, { userOrgId }) =>
        query
            .from({ mail_thread_state: threadStateCollection })
            .where(({ mail_thread_state }) => eq(mail_thread_state.user_org, userOrgId))
    )

    const folderCounts = useMemo(() => {
        const states = threadStates ?? []
        return {
            inbox: states.filter((s) => s.folder === 'inbox' && !s.is_read).length,
            drafts: states.filter((s) => s.folder === 'drafts').length,
            sent: states.filter((s) => s.folder === 'sent').length,
            starred: states.filter((s) => s.is_starred).length,
            trash: states.filter((s) => s.folder === 'trash').length,
            spam: states.filter((s) => s.folder === 'spam').length,
        }
    }, [threadStates])

    const navigateToFolder = (folder: string) => {
        if (folder === 'inbox') {
            router.push(orgHref('mail'))
        } else {
            router.push(orgHref('mail', { folder }))
        }
    }

    const toggleLabel = (labelId: string) => {
        const next = new Set(activeLabels)
        if (next.has(labelId)) {
            next.delete(labelId)
        } else {
            next.add(labelId)
        }
        if (next.size === 0) {
            router.push(orgHref('mail'))
        } else {
            router.push(orgHref('mail', { label: Array.from(next).join(',') }))
        }
    }

    const labelItems = orgLabels.map((label) => (
        <SidebarItem
            key={label.id}
            label={label.name}
            colorDot={label.color}
            isActive={activeLabels.has(label.id)}
            closesDrawer
            onPress={() => toggleLabel(label.id)}
        />
    ))

    return (
        <SidebarNav>
            <SidebarActionButton label="Compose" icon={Pencil} onPress={() => composeEvents.emit()} />

            <SidebarItem
                label="Inbox"
                icon={Inbox}
                badge={folderCounts.inbox || undefined}
                isActive={activeFolder === 'inbox'}
                closesDrawer
                onPress={() => navigateToFolder('inbox')}
            />
            <SidebarItem
                label="Starred"
                icon={Star}
                isActive={activeFolder === 'starred'}
                closesDrawer
                onPress={() => navigateToFolder('starred')}
            />
            <SidebarItem
                label="Sent"
                icon={Send}
                isActive={activeFolder === 'sent'}
                closesDrawer
                onPress={() => navigateToFolder('sent')}
            />
            <SidebarItem
                label="Drafts"
                icon={File}
                badge={folderCounts.drafts || undefined}
                isActive={activeFolder === 'drafts'}
                closesDrawer
                onPress={() => navigateToFolder('drafts')}
            />
            <SidebarItem
                label="All Mail"
                icon={Mail}
                isActive={activeFolder === 'all'}
                closesDrawer
                onPress={() => navigateToFolder('all')}
            />
            <SidebarItem
                label="Spam"
                icon={AlertTriangle}
                badge={folderCounts.spam || undefined}
                isActive={activeFolder === 'spam'}
                closesDrawer
                onPress={() => navigateToFolder('spam')}
            />
            <SidebarItem
                label="Trash"
                icon={Trash2}
                badge={folderCounts.trash || undefined}
                isActive={activeFolder === 'trash'}
                closesDrawer
                onPress={() => navigateToFolder('trash')}
            />
            <SidebarItem
                label="Archive"
                icon={Archive}
                isActive={activeFolder === 'archive'}
                closesDrawer
                onPress={() => navigateToFolder('archive')}
            />

            <SidebarDivider />

            <SidebarHeading
                action={
                    <Pressable
                        onPress={() => setLabelManagerOpen(true)}
                        hitSlop={{ top: 8, bottom: 8, left: 8, right: 8 }}
                    >
                        <Settings size={14} color={mutedColor} />
                    </Pressable>
                }
            >
                Labels
            </SidebarHeading>

            {labelItems}

            <LabelManagerDialog isVisible={labelManagerOpen} onClose={() => setLabelManagerOpen(false)} />
        </SidebarNav>
    )
}
