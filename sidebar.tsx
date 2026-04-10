import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import {
    AlertTriangle,
    Archive,
    ChevronDown,
    ChevronRight,
    File,
    Inbox,
    Mail,
    Pencil,
    Send,
    Star,
    Tag,
    Trash2,
} from 'lucide-react-native'
import { useActiveParams, usePathname, useRouter } from 'one'
import { useMemo, useState } from 'react'
import { Pressable, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
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
import { LabelCreateDialog } from './components/LabelCreateDialog'
import { composeEvents } from './hooks/composeEvents'
import { useLabels } from './hooks/useLabels'

interface MailSidebarProps {
    isCollapsed: boolean
}

function useActiveFolder() {
    const pathname = usePathname()
    const { folder, label } = useActiveParams<{ folder?: string; label?: string }>()
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
    const [moreExpanded, setMoreExpanded] = useState(false)
    const [labelDialogOpen, setLabelDialogOpen] = useState(false)

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

    const MoreIcon = moreExpanded ? ChevronDown : ChevronRight

    const labelItems = orgLabels.map(label => (
        <SidebarItem
            key={label.id}
            label={label.name}
            icon={Tag}
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
                label="More"
                icon={MoreIcon}
                isActive={false}
                onPress={() => setMoreExpanded(prev => !prev)}
            />
            {moreExpanded ? (
                <>
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
                </>
            ) : null}

            <SidebarDivider />

            <View style={styles.labelsHeader}>
                <SidebarHeading>Labels</SidebarHeading>
                <Pressable style={styles.addLabelButton} onPress={() => setLabelDialogOpen(true)}>
                    <Text style={[styles.addLabelText, { color: theme.color8.val }]}>+</Text>
                </Pressable>
            </View>

            {labelItems}

            <LabelCreateDialog
                isVisible={labelDialogOpen}
                onClose={() => setLabelDialogOpen(false)}
            />
        </SidebarNav>
    )
}

const styles = StyleSheet.create({
    labelsHeader: {
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'space-between',
        paddingRight: 12,
    },
    addLabelButton: {
        padding: 4,
    },
    addLabelText: {
        fontSize: 18,
        fontWeight: '600',
    },
})
