import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useLocalSearchParams, usePathname, useRouter } from 'expo-router'
import {
    AlertTriangle,
    Archive,
    File,
    Inbox,
    Mail,
    Pencil,
    Send,
    Settings,
    Star,
    Trash2,
} from 'lucide-react-native'
import { useMemo, useState } from 'react'
import { Pressable } from 'react-native'
import { useTheme } from 'tamagui'
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
import { useCurrentRole } from '~/lib/use-current-role'
import { composeEvents } from './hooks/composeEvents'
import { useLabels } from './hooks/useLabels'

interface MailSidebarProps {
    isCollapsed: boolean
}

function useActiveFolder() {
    const pathname = usePathname()
    const { folder, label } = useLocalSearchParams<{ folder?: string; label?: string }>()
    if (pathname.includes('/mail/')) return null
    if (label) return `label:${label}`
    return folder ?? 'inbox'
}

export default function MailSidebar(_props: MailSidebarProps) {
    const router = useRouter()
    const theme = useTheme()
    const activeFolder = useActiveFolder()
    const { userOrgId } = useCurrentRole()
    const orgHref = useOrgHref()
    const [labelManagerOpen, setLabelManagerOpen] = useState(false)

    const [threadStateCollection] = useStore('mail_thread_state')
    const { labels: orgLabels } = useLabels()

    const { data: threadStates } = useLiveQuery(
        query =>
            query
                .from({ mail_thread_state: threadStateCollection })
                .where(({ mail_thread_state }) => eq(mail_thread_state.user_org, userOrgId)),
        [userOrgId]
    )

    const folderCounts = useMemo(() => {
        const states = threadStates ?? []
        return {
            inbox: states.filter(s => s.folder === 'inbox' && !s.is_read).length,
            drafts: states.filter(s => s.folder === 'drafts').length,
            sent: states.filter(s => s.folder === 'sent').length,
            starred: states.filter(s => s.is_starred).length,
            trash: states.filter(s => s.folder === 'trash').length,
            spam: states.filter(s => s.folder === 'spam').length,
        }
    }, [threadStates])

    const navigateToFolder = (folder: string) => {
        if (folder === 'inbox') {
            router.push(orgHref('mail'))
        } else {
            router.push(orgHref('mail', { folder }))
        }
    }

    const navigateToLabel = (labelId: string) => {
        router.push(orgHref('mail', { label: labelId }))
    }

    const labelItems = orgLabels.map(label => (
        <SidebarItem
            key={label.id}
            label={label.name}
            colorDot={label.color}
            isActive={activeFolder === `label:${label.id}`}
            onPress={() => navigateToLabel(label.id)}
        />
    ))

    return (
        <SidebarNav>
            <SidebarActionButton
                label="Compose"
                icon={Pencil}
                onPress={() => composeEvents.emit()}
            />

            <SidebarItem
                label="Inbox"
                icon={Inbox}
                badge={folderCounts.inbox || undefined}
                isActive={activeFolder === 'inbox'}
                onPress={() => navigateToFolder('inbox')}
            />
            <SidebarItem
                label="Starred"
                icon={Star}
                isActive={activeFolder === 'starred'}
                onPress={() => navigateToFolder('starred')}
            />
            <SidebarItem
                label="Sent"
                icon={Send}
                isActive={activeFolder === 'sent'}
                onPress={() => navigateToFolder('sent')}
            />
            <SidebarItem
                label="Drafts"
                icon={File}
                badge={folderCounts.drafts || undefined}
                isActive={activeFolder === 'drafts'}
                onPress={() => navigateToFolder('drafts')}
            />
            <SidebarItem
                label="All Mail"
                icon={Mail}
                isActive={activeFolder === 'all'}
                onPress={() => navigateToFolder('all')}
            />
            <SidebarItem
                label="Spam"
                icon={AlertTriangle}
                badge={folderCounts.spam || undefined}
                isActive={activeFolder === 'spam'}
                onPress={() => navigateToFolder('spam')}
            />
            <SidebarItem
                label="Trash"
                icon={Trash2}
                badge={folderCounts.trash || undefined}
                isActive={activeFolder === 'trash'}
                onPress={() => navigateToFolder('trash')}
            />
            <SidebarItem
                label="Archive"
                icon={Archive}
                isActive={activeFolder === 'archive'}
                onPress={() => navigateToFolder('archive')}
            />

            <SidebarDivider />

            <SidebarHeading
                action={
                    <Pressable
                        onPress={() => setLabelManagerOpen(true)}
                        hitSlop={{ top: 8, bottom: 8, left: 8, right: 8 }}
                    >
                        <Settings size={14} color={theme.color8.val} />
                    </Pressable>
                }
            >
                Labels
            </SidebarHeading>

            {labelItems}

            <LabelManagerDialog
                isVisible={labelManagerOpen}
                onClose={() => setLabelManagerOpen(false)}
            />
        </SidebarNav>
    )
}
