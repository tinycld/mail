import { useThemeColor } from 'heroui-native'
import { Maximize2, Minimize2, X } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { useBreakpoint } from '~/components/workspace/useBreakpoint'
import type { ComposeMode } from '../hooks/useComposeState'

interface ComposeHeaderProps {
    mode: ComposeMode
    title?: string
    onMinimize: () => void
    onMaximize: () => void
    onClose: () => void
}

export function ComposeHeader({
    mode,
    title,
    onMinimize,
    onMaximize,
    onClose,
}: ComposeHeaderProps) {
    const [foregroundColor, backgroundColor] = useThemeColor(['foreground', 'background'])
    const breakpoint = useBreakpoint()
    const showWindowControls = breakpoint === 'desktop'
    const displayTitle = title?.trim() || 'New Message'

    return (
        <View
            style={{
                flexDirection: 'row',
                alignItems: 'center',
                justifyContent: 'space-between',
                height: 40,
                paddingHorizontal: 12,
                borderTopLeftRadius: 8,
                borderTopRightRadius: 8,
                backgroundColor: foregroundColor,
            }}
        >
            <Pressable style={{ flex: 1 }} onPress={mode === 'minimized' ? onMinimize : undefined}>
                <Text
                    style={{ fontSize: 14, fontWeight: '600', color: backgroundColor }}
                    numberOfLines={1}
                >
                    {displayTitle}
                </Text>
            </Pressable>
            <View style={{ flexDirection: 'row', alignItems: 'center', gap: 4 }}>
                {showWindowControls ? (
                    <>
                        <Pressable style={{ padding: 6, borderRadius: 4 }} onPress={onMinimize}>
                            <Minimize2 size={14} color={backgroundColor} />
                        </Pressable>
                        <Pressable style={{ padding: 6, borderRadius: 4 }} onPress={onMaximize}>
                            <Maximize2 size={14} color={backgroundColor} />
                        </Pressable>
                    </>
                ) : null}
                <Pressable style={{ padding: 6, borderRadius: 4 }} onPress={onClose}>
                    <X size={14} color={backgroundColor} />
                </Pressable>
            </View>
        </View>
    )
}
