import { ArrowLeft } from 'lucide-react-native'
import { useParams, useRouter } from 'one'
import { useMemo } from 'react'
import { Pressable } from 'react-native'
import { SizableText, useTheme, XStack, YStack } from 'tamagui'
import { addonSettings } from '~/lib/generated/addon-settings'
import { useOrgHref } from '~/lib/org-routes'
import { useCurrentRole } from '~/lib/use-current-role'

export default function AddonSettingsSection() {
    const router = useRouter()
    const theme = useTheme()
    const { isAdmin } = useCurrentRole()
    const orgHref = useOrgHref()
    const params = useParams<{ section: string[] }>()
    const segments = params.section ?? []
    const [addonSlug, panelSlug] = segments

    const match = useMemo(() => {
        if (!addonSlug || !panelSlug) return null
        const group = addonSettings.find(g => g.addonSlug === addonSlug)
        if (!group) return null
        const panel = group.panels.find(p => p.slug === panelSlug)
        if (!panel) return null
        return { group, panel }
    }, [addonSlug, panelSlug])

    if (!isAdmin) {
        return (
            <YStack
                flex={1}
                padding="$5"
                alignItems="center"
                justifyContent="center"
                backgroundColor="$background"
            >
                <SizableText size="$5" color="$color8">
                    Only admins can access addon settings.
                </SizableText>
            </YStack>
        )
    }

    if (!match) {
        return (
            <YStack
                flex={1}
                padding="$5"
                alignItems="center"
                justifyContent="center"
                backgroundColor="$background"
            >
                <SizableText size="$6" fontWeight="bold" color="$color" marginBottom="$3">
                    Settings not found
                </SizableText>
                <Pressable onPress={() => router.push(orgHref('settings'))}>
                    <SizableText size="$4" color="$accentColor">
                        Back to Settings
                    </SizableText>
                </Pressable>
            </YStack>
        )
    }

    const { group, panel } = match
    const PanelComponent = panel.Component

    return (
        <YStack flex={1} backgroundColor="$background">
            <XStack gap="$3" alignItems="center" padding="$5" paddingBottom="$0">
                <Pressable onPress={() => router.back()}>
                    <ArrowLeft size={24} color={theme.color.val} />
                </Pressable>
                <SizableText size="$7" fontWeight="bold" color="$color">
                    {panel.label}
                </SizableText>
                <SizableText size="$4" color="$color8">
                    {group.addonName}
                </SizableText>
            </XStack>
            <YStack flex={1}>
                <PanelComponent />
            </YStack>
        </YStack>
    )
}
