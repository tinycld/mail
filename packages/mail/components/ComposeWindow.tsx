import { Platform, StyleSheet, TextInput, View } from 'react-native'
import { useTheme } from 'tamagui'
import { useBreakpoint } from '~/components/workspace/useBreakpoint'
import { useCompose } from '../hooks/useComposeState'
import { ComposeFields } from './ComposeFields'
import { ComposeHeader } from './ComposeHeader'
import { ComposeToolbar } from './ComposeToolbar'

const webShadow =
    Platform.OS === 'web'
        ? ({ boxShadow: '0 8px 32px rgba(0,0,0,0.24)' } as Record<string, unknown>)
        : {}

interface ComposeWindowProps {
    isVisible: boolean
}

export function ComposeWindow({ isVisible }: ComposeWindowProps) {
    const theme = useTheme()
    const { mode, minimize, maximize, open, close } = useCompose()
    const breakpoint = useBreakpoint()

    if (!isVisible) return null

    const isMinimized = mode === 'minimized'
    const isMaximized = mode === 'maximized'
    const isNotDesktop = breakpoint !== 'desktop'

    const modeStyleMap = {
        open: styles.normal,
        minimized: styles.minimized,
        maximized: styles.maximized,
        closed: styles.normal,
    }
    const windowStyle = isNotDesktop ? styles.fullscreen : modeStyleMap[mode]

    return (
        <View
            style={[
                styles.container,
                windowStyle,
                {
                    backgroundColor: theme.background.val,
                    borderColor: theme.borderColor.val,
                    ...webShadow,
                },
            ]}
        >
            <ComposeHeader
                mode={mode}
                onMinimize={isMinimized ? open : minimize}
                onMaximize={isMaximized ? open : maximize}
                onClose={close}
            />
            {isMinimized ? null : (
                <>
                    <ComposeFields />
                    <View style={styles.body}>
                        <TextInput
                            style={[styles.bodyInput, { color: theme.color.val }]}
                            multiline
                            placeholder="Compose email"
                            placeholderTextColor={theme.placeholderColor.val}
                        />
                    </View>
                    <ComposeToolbar onDiscard={close} />
                </>
            )}
        </View>
    )
}

const styles = StyleSheet.create({
    container: {
        position: 'absolute',
        borderWidth: 1,
        borderRadius: 8,
        overflow: 'hidden',
        zIndex: 1000,
    },
    normal: {
        bottom: 0,
        right: 16,
        width: 500,
        height: 560,
    },
    minimized: {
        bottom: 0,
        right: 16,
        width: 300,
        height: 40,
    },
    maximized: {
        bottom: 0,
        right: 16,
        width: 700,
        height: 700,
    },
    fullscreen: {
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
    },
    body: {
        flex: 1,
        padding: 12,
    },
    bodyInput: {
        flex: 1,
        fontSize: 14,
        textAlignVertical: 'top',
    },
})
