import { Suspense } from 'react'
import { Pressable, StyleSheet, View } from 'react-native'
import Animated, { useAnimatedStyle, useDerivedValue, withSpring } from 'react-native-reanimated'
import { useTheme } from 'tamagui'
import { useAddon } from '~/lib/addons/use-addons'
import { addonSidebars } from '~/lib/generated/addon-sidebars'
import { AddonSidebarFallback } from './AddonSidebarFallback'
import { useWorkspaceLayout } from './useWorkspaceLayout'

const PANEL_WIDTH = 280

const SPRING_CONFIG = {
    damping: 25,
    stiffness: 200,
    mass: 0.8,
}

interface MobileDrawerProps {
    isVisible: boolean
}

export function MobileDrawer({ isVisible }: MobileDrawerProps) {
    const theme = useTheme()
    const { activeAddonSlug, setDrawerOpen } = useWorkspaceLayout()
    const addon = useAddon(activeAddonSlug ?? '')

    const progress = useDerivedValue(() => withSpring(isVisible ? 1 : 0, SPRING_CONFIG))

    const panelStyle = useAnimatedStyle(() => ({
        transform: [{ translateX: -PANEL_WIDTH + progress.value * PANEL_WIDTH }],
    }))

    const backdropStyle = useAnimatedStyle(() => ({
        opacity: progress.value,
    }))

    const basePath = addon ? `/app/${addon.slug}` : '/app'
    const SidebarComponent = addon ? addonSidebars[addon.slug] : null

    return (
        <View style={styles.overlay} pointerEvents={isVisible ? 'auto' : 'none'}>
            <Animated.View style={[styles.backdrop, backdropStyle]}>
                <Pressable
                    style={[styles.backdropPress, { backgroundColor: theme.overlayBackground.val }]}
                    onPress={() => setDrawerOpen(false)}
                />
            </Animated.View>
            <Animated.View
                style={[styles.panel, panelStyle, { backgroundColor: theme.sidebarBackground.val }]}
            >
                {SidebarComponent ? (
                    <Suspense fallback={null}>
                        <SidebarComponent basePath={basePath} isCollapsed={false} />
                    </Suspense>
                ) : (
                    <AddonSidebarFallback
                        addonLabel={addon?.nav.label ?? 'Menu'}
                        basePath={basePath}
                    />
                )}
            </Animated.View>
        </View>
    )
}

const styles = StyleSheet.create({
    overlay: {
        position: 'absolute',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        zIndex: 200,
    },
    backdrop: {
        position: 'absolute',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
    },
    backdropPress: {
        flex: 1,
    },
    panel: {
        position: 'absolute',
        top: 0,
        left: 0,
        bottom: 0,
        width: PANEL_WIDTH,
        zIndex: 201,
    },
})
