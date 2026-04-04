import { Building2, type LucideIcon, Settings, User } from 'lucide-react-native'
import type { OneRouter } from 'one'
import { useRouter } from 'one'
import { Pressable, StyleSheet, View } from 'react-native'
import { useTheme } from 'tamagui'
import { useAddons } from '~/lib/addons/use-addons'
import { useAuth } from '~/lib/auth'
import { getIcon } from './addon-icon-map'
import { useWorkspaceLayout } from './useWorkspaceLayout'

export function AddonRail() {
    const theme = useTheme()
    const addons = useAddons()
    const { activeAddonSlug } = useWorkspaceLayout()
    const { logout } = useAuth()
    const router = useRouter()

    const sorted = [...addons].sort((a, b) => (a.nav.order ?? 99) - (b.nav.order ?? 99))

    return (
        <View style={[styles.rail, { backgroundColor: theme.railBackground.val }]}>
            <View style={styles.topSection}>
                <Pressable
                    onPress={() => router.push('/app' as OneRouter.Href)}
                    style={styles.railItem}
                    accessibilityLabel="Organization home"
                >
                    <Building2 size={24} color={theme.railText.val} />
                </Pressable>

                <View style={[styles.divider, { backgroundColor: theme.railText.val }]} />

                {sorted.map(addon => {
                    const Icon = getIcon(addon.nav.icon)
                    const isActive = activeAddonSlug === addon.slug
                    return (
                        <AddonRailItem
                            key={addon.slug}
                            slug={addon.slug}
                            label={addon.nav.label}
                            Icon={Icon}
                            isActive={isActive}
                            activeColor={theme.activeIndicator.val}
                            textColor={isActive ? theme.railActiveText.val : theme.railText.val}
                        />
                    )
                })}
            </View>

            <View style={styles.bottomSection}>
                <Pressable
                    onPress={() => router.push('/app/settings' as OneRouter.Href)}
                    style={styles.railItem}
                    accessibilityLabel="Settings"
                >
                    <Settings size={22} color={theme.railText.val} />
                </Pressable>

                <Pressable onPress={logout} style={styles.avatar} accessibilityLabel="Sign out">
                    <User size={20} color={theme.railActiveText.val} />
                </Pressable>
            </View>
        </View>
    )
}

function AddonRailItem({
    slug,
    label,
    Icon,
    isActive,
    activeColor,
    textColor,
}: {
    slug: string
    label: string
    Icon: LucideIcon
    isActive: boolean
    activeColor: string
    textColor: string
}) {
    const router = useRouter()

    return (
        <Pressable
            onPress={() => router.push(`/app/${slug}` as OneRouter.Href)}
            style={[styles.railItem, isActive && { backgroundColor: `${activeColor}22` }]}
            accessibilityLabel={label}
        >
            {isActive && (
                <View style={[styles.activeIndicator, { backgroundColor: activeColor }]} />
            )}
            <Icon size={22} color={textColor} />
        </Pressable>
    )
}

const styles = StyleSheet.create({
    rail: {
        width: 64,
        flexDirection: 'column',
        justifyContent: 'space-between',
        alignItems: 'center',
        paddingVertical: 12,
    },
    topSection: {
        alignItems: 'center',
        gap: 4,
    },
    bottomSection: {
        alignItems: 'center',
        gap: 8,
    },
    railItem: {
        width: 44,
        height: 44,
        borderRadius: 12,
        justifyContent: 'center',
        alignItems: 'center',
        position: 'relative',
    },
    activeIndicator: {
        position: 'absolute',
        left: -8,
        width: 4,
        height: 20,
        borderRadius: 2,
    },
    divider: {
        width: 28,
        height: 1,
        opacity: 0.2,
        marginVertical: 8,
    },
    avatar: {
        width: 32,
        height: 32,
        borderRadius: 16,
        backgroundColor: 'rgba(255,255,255,0.15)',
        justifyContent: 'center',
        alignItems: 'center',
    },
})
