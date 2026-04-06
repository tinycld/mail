import { Slot } from 'one'
import { Platform, StyleSheet, View } from 'react-native'
import { useTheme } from 'tamagui'
import { AddonProviderWrapper } from './AddonProviderWrapper'
import { MobileDrawer } from './MobileDrawer'
import { MobileTabBar } from './MobileTabBar'
import { useWorkspaceLayout } from './useWorkspaceLayout'

export function MobileLayout({ isReady = true }: { isReady?: boolean }) {
    const theme = useTheme()
    const { isDrawerOpen } = useWorkspaceLayout()

    return (
        <AddonProviderWrapper>
            <View style={[styles.container, { backgroundColor: theme.background.val }]}>
                <View style={styles.content}>
                    <Slot />
                </View>
                {isReady && <MobileTabBar />}
                {isReady && <MobileDrawer isVisible={isDrawerOpen} />}
            </View>
        </AddonProviderWrapper>
    )
}

const styles = StyleSheet.create({
    container: {
        flex: 1,
        ...(Platform.OS === 'web' ? { height: '100vh' as unknown as number } : {}),
    },
    content: {
        flex: 1,
        overflow: 'hidden',
    },
})
