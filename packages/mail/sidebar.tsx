import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { ChevronDown, Clock, File, Inbox, Pencil, Send, Star, Tag } from 'lucide-react-native'
import { useActiveParams, usePathname, useRouter } from 'one'
import { useMemo } from 'react'
import { Pressable, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import {
    SidebarDivider,
    SidebarHeading,
    SidebarItem,
    SidebarNav,
} from '~/components/sidebar-primitives'
import { useOrgHref } from '~/lib/org-routes'
import { useStore } from '~/lib/pocketbase'
import { useCurrentRole } from '~/lib/use-current-role'
import { useOrgInfo } from '~/lib/use-org-info'
import { composeEvents } from './hooks/composeEvents'

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
    const { orgId } = useOrgInfo()
    const orgHref = useOrgHref()

    const [threadStateCollection, labelsCollection] = useStore('mail_thread_state', 'mail_labels')

    const { data: threadStates } = useLiveQuery(
        query =>
            query
                .from({ mail_thread_state: threadStateCollection })
                .where(({ mail_thread_state }) => eq(mail_thread_state.user_org, userOrgId)),
        [userOrgId]
    )

    const { data: orgLabels } = useLiveQuery(
        query =>
            query
                .from({ mail_labels: labelsCollection })
                .where(({ mail_labels }) => eq(mail_labels.org, orgId)),
        [orgId]
    )

    const folderCounts = useMemo(() => {
        const states = threadStates ?? []
        return {
            inbox: states.filter(s => s.folder === 'inbox' && !s.is_read).length,
            drafts: states.filter(s => s.folder === 'drafts').length,
            sent: states.filter(s => s.folder === 'sent').length,
            starred: states.filter(s => s.is_starred).length,
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

    const labelItems = (orgLabels ?? []).map(label => (
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
            <View style={styles.composeWrapper}>
                <Pressable
                    style={[styles.composeButton, { backgroundColor: theme.accentBackground.val }]}
                    onPress={() => composeEvents.emit()}
                >
                    <Pencil size={16} color={theme.accentColor.val} />
                    <Text style={[styles.composeText, { color: theme.accentColor.val }]}>
                        Compose
                    </Text>
                </Pressable>
            </View>

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
                label="Snoozed"
                icon={Clock}
                isActive={activeFolder === 'snoozed'}
                onPress={() => navigateToFolder('snoozed')}
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
            <SidebarItem label="Categories" icon={ChevronDown} isActive={false} />
            <SidebarItem label="More" icon={ChevronDown} isActive={false} />

            <SidebarDivider />

            <View style={styles.labelsHeader}>
                <SidebarHeading>Labels</SidebarHeading>
                <Pressable style={styles.addLabelButton}>
                    <Text style={[styles.addLabelText, { color: theme.color8.val }]}>+</Text>
                </Pressable>
            </View>

            {labelItems}
        </SidebarNav>
    )
}

const styles = StyleSheet.create({
    composeWrapper: {
        paddingHorizontal: 12,
        paddingVertical: 8,
    },
    composeButton: {
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'center',
        gap: 8,
        paddingHorizontal: 20,
        paddingVertical: 12,
        borderRadius: 24,
    },
    composeText: {
        fontSize: 14,
        fontWeight: '600',
    },
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
