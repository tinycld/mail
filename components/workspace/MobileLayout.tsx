import { Slot } from 'one'
import { Platform, StyleSheet, View } from 'react-native'
import { useTheme } from 'tamagui'
import { MobileDrawer } from './MobileDrawer'
import { MobileTabBar } from './MobileTabBar'
import { useWorkspaceLayout } from './useWorkspaceLayout'

export function MobileLayout() {
    const theme = useTheme()
    const { isDrawerOpen } = useWorkspaceLayout()

    return (
        <View style={[styles.container, { backgroundColor: theme.background.val }]}>
            <View style={styles.content}>
                <Slot />
            </View>
            <MobileTabBar />
            <MobileDrawer isVisible={isDrawerOpen} />
        </View>
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
