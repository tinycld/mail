import { Building2, ChevronRight, User, Users } from 'lucide-react-native'
import type { OneRouter } from 'one'
import { useRouter } from 'one'
import { Pressable, StyleSheet, View } from 'react-native'
import { ScrollView, SizableText, useTheme } from 'tamagui'
import { getIcon } from '~/components/workspace/addon-icon-map'
import { addonSettings } from '~/lib/generated/addon-settings'
import { useCurrentRole } from '~/lib/use-current-role'

export default function SettingsIndex() {
    const theme = useTheme()
    const { isAdmin } = useCurrentRole()

    return (
        <ScrollView flex={1} backgroundColor="$background" contentContainerStyle={{ flexGrow: 1 }}>
            <View style={styles.container}>
                <SizableText size="$8" fontWeight="bold" color="$color" marginBottom="$4">
                    Settings
                </SizableText>

                <SettingsGroup label="Account">
                    <SettingsLink
                        label="Profile"
                        href="/app/settings/profile"
                        icon={<User size={20} color={theme.color.val} />}
                    />
                </SettingsGroup>

                <AdminSettings isVisible={isAdmin} />
            </View>
        </ScrollView>
    )
}

function AdminSettings({ isVisible }: { isVisible: boolean }) {
    const theme = useTheme()

    if (!isVisible) return null

    return (
        <>
            <SettingsGroup label="Organization">
                <SettingsLink
                    label="General"
                    href="/app/settings/organization"
                    icon={<Building2 size={20} color={theme.color.val} />}
                />
                <SettingsLink
                    label="Members"
                    href="/app/settings/members"
                    icon={<Users size={20} color={theme.color.val} />}
                />
            </SettingsGroup>

            {addonSettings.map(group => {
                const Icon = getIcon(group.addonSlug)
                return (
                    <SettingsGroup key={group.addonSlug} label={group.addonName}>
                        {group.panels.map(panel => (
                            <SettingsLink
                                key={panel.slug}
                                label={panel.label}
                                href={`/app/settings/${group.addonSlug}/${panel.slug}`}
                                icon={<Icon size={20} color={theme.color.val} />}
                            />
                        ))}
                    </SettingsGroup>
                )
            })}
        </>
    )
}

function SettingsGroup({ label, children }: { label: string; children: React.ReactNode }) {
    const theme = useTheme()

    return (
        <View style={styles.group}>
            <SizableText
                size="$3"
                fontWeight="600"
                color="$colorFocus"
                textTransform="uppercase"
                letterSpacing={0.5}
                marginBottom="$2"
            >
                {label}
            </SizableText>
            <View
                style={[
                    styles.groupContent,
                    {
                        backgroundColor: theme.backgroundHover.val,
                        borderColor: theme.borderColor.val,
                    },
                ]}
            >
                {children}
            </View>
        </View>
    )
}

function SettingsLink({
    label,
    href,
    icon,
}: {
    label: string
    href: string
    icon: React.ReactNode
}) {
    const theme = useTheme()
    const router = useRouter()

    return (
        <Pressable
            onPress={() => router.push(href as OneRouter.Href)}
            style={({ pressed }) => [styles.row, pressed && { opacity: 0.7 }]}
        >
            <View style={styles.rowLeft}>
                {icon}
                <SizableText size="$5" color="$color">
                    {label}
                </SizableText>
            </View>
            <ChevronRight size={18} color={theme.color8.val} />
        </Pressable>
    )
}

const styles = StyleSheet.create({
    container: {
        padding: 24,
        maxWidth: 600,
        width: '100%',
    },
    group: {
        marginBottom: 24,
    },
    groupContent: {
        borderRadius: 12,
        borderWidth: 1,
        overflow: 'hidden',
    },
    row: {
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'space-between',
        paddingHorizontal: 16,
        paddingVertical: 14,
    },
    rowLeft: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 12,
    },
})
