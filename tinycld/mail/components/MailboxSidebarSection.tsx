import { SidebarItem } from '@tinycld/core/components/sidebar-primitives'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import {
    AlertTriangle,
    Archive,
    AtSign,
    ChevronDown,
    ChevronRight,
    File,
    Inbox,
    Mail,
    Send,
    Star,
    Trash2,
} from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { useAddressViews } from '../hooks/useAddressViews'
import type { FolderCounts } from '../hooks/useMailboxFolderCounts'
import { addressViewFolder } from '../lib/thread-list-query'
import { useMailSidebarStore } from '../stores/sidebar-store'

interface AddressViewItemsProps {
    isVisible: boolean
    mailboxId: string
    isActive: (folder: string) => boolean
    onNavigate: (mailboxId: string, folder: string) => void
}

// An empty, zero-size marker that tells e2e the views query has loaded, as
// package-sidebar-mounted does for the sidebar: with it, no entries is an
// answer and not a query still loading.
function ViewsReadyMarker({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return <View testID="address-views-ready" />
}

function AddressViewList({ mailboxId, isActive, onNavigate }: AddressViewItemsProps) {
    const { views, isReady } = useAddressViews()
    const items = views.map(view => {
        const folder = addressViewFolder(view.id)
        return (
            <SidebarItem
                key={view.id}
                testID={`address-view-${view.email}`}
                label={view.email}
                icon={AtSign}
                badge={view.unread || undefined}
                isActive={isActive(folder)}
                closesDrawer
                onPress={() => onNavigate(mailboxId, folder)}
            />
        )
    })
    return (
        <View className="pl-4">
            <ViewsReadyMarker isVisible={isReady} />
            {items}
        </View>
    )
}

/** A folder for each address of the personal inbox, under its Inbox item. */
function AddressViewItems(props: AddressViewItemsProps) {
    if (!props.isVisible) return null
    return <AddressViewList {...props} />
}

interface Props {
    mailboxId: string
    mailboxLabel: string
    /** The personal inbox, the only one with address views. */
    isPersonal?: boolean
    defaultExpanded: boolean
    counts: FolderCounts
    activeMailboxId: string
    activeFolder: string | null
    onNavigate: (mailboxId: string, folder: string) => void
}

export function MailboxSidebarSection({
    mailboxId,
    mailboxLabel,
    isPersonal = false,
    defaultExpanded,
    counts,
    activeMailboxId,
    activeFolder,
    onNavigate,
}: Props) {
    const fgColor = useThemeColor('foreground')
    const mutedColor = useThemeColor('muted-foreground')

    const isExpanded = useMailSidebarStore(s => s.isExpanded(mailboxId, defaultExpanded))
    const toggle = useMailSidebarStore(s => s.toggle)

    const aggregateUnread = counts.inbox

    const isActive = (folder: string) => activeMailboxId === mailboxId && activeFolder === folder

    const handleHeaderPress = () => {
        toggle(mailboxId, defaultExpanded)
        if (!isExpanded) onNavigate(mailboxId, 'inbox')
    }

    return (
        <View testID="mailbox-section">
            <Pressable
                onPress={handleHeaderPress}
                className="flex-row gap-1 items-center pl-1 pr-3 py-2"
            >
                {isExpanded ? (
                    <ChevronDown size={14} color={mutedColor} />
                ) : (
                    <ChevronRight size={14} color={mutedColor} />
                )}
                <Text
                    className="flex-1"
                    style={{ fontSize: 13, fontWeight: '600', color: fgColor }}
                >
                    {mailboxLabel}
                </Text>
                {!isExpanded && aggregateUnread > 0 && (
                    <Text style={{ fontSize: 12, color: mutedColor }}>{aggregateUnread}</Text>
                )}
            </Pressable>

            {isExpanded && (
                <View>
                    <SidebarItem
                        label="Inbox"
                        icon={Inbox}
                        badge={counts.inbox || undefined}
                        isActive={isActive('inbox')}
                        closesDrawer
                        onPress={() => onNavigate(mailboxId, 'inbox')}
                    />
                    <AddressViewItems
                        isVisible={isPersonal}
                        mailboxId={mailboxId}
                        isActive={isActive}
                        onNavigate={onNavigate}
                    />
                    <SidebarItem
                        label="Starred"
                        icon={Star}
                        isActive={isActive('starred')}
                        closesDrawer
                        onPress={() => onNavigate(mailboxId, 'starred')}
                    />
                    <SidebarItem
                        label="Sent"
                        icon={Send}
                        isActive={isActive('sent')}
                        closesDrawer
                        onPress={() => onNavigate(mailboxId, 'sent')}
                    />
                    <SidebarItem
                        label="Drafts"
                        icon={File}
                        badge={counts.drafts || undefined}
                        isActive={isActive('drafts')}
                        closesDrawer
                        onPress={() => onNavigate(mailboxId, 'drafts')}
                    />
                    <SidebarItem
                        label="All Mail"
                        icon={Mail}
                        isActive={isActive('all')}
                        closesDrawer
                        onPress={() => onNavigate(mailboxId, 'all')}
                    />
                    <SidebarItem
                        label="Spam"
                        icon={AlertTriangle}
                        badge={counts.spam || undefined}
                        isActive={isActive('spam')}
                        closesDrawer
                        onPress={() => onNavigate(mailboxId, 'spam')}
                    />
                    <SidebarItem
                        label="Trash"
                        icon={Trash2}
                        badge={counts.trash || undefined}
                        isActive={isActive('trash')}
                        closesDrawer
                        onPress={() => onNavigate(mailboxId, 'trash')}
                    />
                    <SidebarItem
                        label="Archive"
                        icon={Archive}
                        isActive={isActive('archive')}
                        closesDrawer
                        onPress={() => onNavigate(mailboxId, 'archive')}
                    />
                </View>
            )}
        </View>
    )
}
