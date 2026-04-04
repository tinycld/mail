import { useEffect, useRef } from 'react'
import { Animated, Platform, StyleSheet, View } from 'react-native'
import { useTheme } from 'tamagui'
import { useBreakpoint } from './useBreakpoint'

function SkeletonBlock({
    width,
    height,
    style,
}: {
    width: number | string
    height: number
    style?: object
}) {
    const theme = useTheme()
    const opacity = useRef(new Animated.Value(0.3)).current

    useEffect(() => {
        const animation = Animated.loop(
            Animated.sequence([
                Animated.timing(opacity, { toValue: 0.7, duration: 800, useNativeDriver: true }),
                Animated.timing(opacity, { toValue: 0.3, duration: 800, useNativeDriver: true }),
            ])
        )
        animation.start()
        return () => animation.stop()
    }, [opacity])

    return (
        <Animated.View
            style={[
                {
                    width: width as number,
                    height,
                    borderRadius: 8,
                    backgroundColor: theme.borderColor.val,
                    opacity,
                },
                style,
            ]}
        />
    )
}

function SkeletonRail() {
    const theme = useTheme()
    return (
        <View style={[skeletonStyles.rail, { backgroundColor: theme.railBackground.val }]}>
            <View style={skeletonStyles.railTop}>
                <SkeletonBlock width={36} height={36} style={{ borderRadius: 10, opacity: 0.15 }} />
                <View
                    style={[skeletonStyles.railDivider, { backgroundColor: theme.railText.val }]}
                />
                <SkeletonBlock width={36} height={36} style={{ borderRadius: 10, opacity: 0.15 }} />
                <SkeletonBlock width={36} height={36} style={{ borderRadius: 10, opacity: 0.15 }} />
                <SkeletonBlock width={36} height={36} style={{ borderRadius: 10, opacity: 0.15 }} />
            </View>
            <View style={skeletonStyles.railBottom}>
                <SkeletonBlock width={36} height={36} style={{ borderRadius: 10, opacity: 0.15 }} />
                <SkeletonBlock width={28} height={28} style={{ borderRadius: 14, opacity: 0.15 }} />
            </View>
        </View>
    )
}

function SkeletonSidebar({ width }: { width: number }) {
    const theme = useTheme()
    return (
        <View
            style={[
                skeletonStyles.sidebar,
                {
                    width,
                    backgroundColor: theme.sidebarBackground.val,
                    borderRightColor: theme.borderColor.val,
                },
            ]}
        >
            <SkeletonBlock width={100} height={11} style={{ marginBottom: 12, marginTop: 20 }} />
            <SkeletonBlock width="85%" height={32} style={{ marginBottom: 4 }} />
            <SkeletonBlock width="85%" height={32} style={{ marginBottom: 4 }} />
            <SkeletonBlock width="85%" height={32} style={{ marginBottom: 16 }} />
            <View
                style={[skeletonStyles.sidebarDivider, { backgroundColor: theme.borderColor.val }]}
            />
            <SkeletonBlock width={80} height={11} style={{ marginBottom: 12, marginTop: 8 }} />
            <SkeletonBlock width="85%" height={32} style={{ marginBottom: 4 }} />
            <SkeletonBlock width="85%" height={32} />
        </View>
    )
}

function SkeletonMain() {
    const theme = useTheme()
    return (
        <View style={[skeletonStyles.main, { backgroundColor: theme.background.val }]}>
            <SkeletonBlock width={200} height={24} style={{ marginBottom: 24 }} />
            <SkeletonBlock width="100%" height={48} style={{ marginBottom: 12 }} />
            <SkeletonBlock width="100%" height={48} style={{ marginBottom: 12 }} />
            <SkeletonBlock width="100%" height={48} style={{ marginBottom: 12 }} />
            <SkeletonBlock width="60%" height={48} />
        </View>
    )
}

const SIDEBAR_WIDTH = 260

function SkeletonTabBar() {
    const theme = useTheme()
    return (
        <View
            style={[
                skeletonStyles.tabBar,
                {
                    backgroundColor: theme.railBackground.val,
                    borderTopColor: theme.borderColor.val,
                },
            ]}
        >
            <SkeletonBlock width={36} height={36} style={{ borderRadius: 10, opacity: 0.15 }} />
            <SkeletonBlock width={36} height={36} style={{ borderRadius: 10, opacity: 0.15 }} />
            <SkeletonBlock width={36} height={36} style={{ borderRadius: 10, opacity: 0.15 }} />
        </View>
    )
}

export function SkeletonLayout() {
    const theme = useTheme()
    const breakpoint = useBreakpoint()

    if (breakpoint === 'mobile') {
        return (
            <View
                style={[
                    skeletonStyles.container,
                    { backgroundColor: theme.background.val, flexDirection: 'column' },
                ]}
            >
                <SkeletonMain />
                <SkeletonTabBar />
            </View>
        )
    }

    return (
        <View style={[skeletonStyles.container, { backgroundColor: theme.background.val }]}>
            <SkeletonRail />
            <SkeletonSidebar width={SIDEBAR_WIDTH} />
            <SkeletonMain />
        </View>
    )
}

const skeletonStyles = StyleSheet.create({
    container: {
        flex: 1,
        flexDirection: 'row',
        ...(Platform.OS === 'web' ? { height: '100vh' as unknown as number } : {}),
    },
    rail: {
        width: 64,
        justifyContent: 'space-between',
        alignItems: 'center',
        paddingVertical: 12,
    },
    railTop: {
        alignItems: 'center',
        gap: 8,
    },
    railBottom: {
        alignItems: 'center',
        gap: 8,
    },
    railDivider: {
        width: 28,
        height: 1,
        opacity: 0.2,
        marginVertical: 4,
    },
    sidebar: {
        borderRightWidth: 1,
        paddingHorizontal: 12,
    },
    sidebarDivider: {
        height: 1,
        marginVertical: 8,
    },
    main: {
        flex: 1,
        padding: 24,
    },
    tabBar: {
        flexDirection: 'row',
        height: 56,
        borderTopWidth: 1,
        alignItems: 'center',
        justifyContent: 'space-around',
        paddingHorizontal: 16,
    },
})
