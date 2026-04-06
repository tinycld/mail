import { Slot } from 'one'
import { View } from 'react-native'
import { XStack, YStack } from 'tamagui'
import { useBreakpoint } from '~/components/workspace/useBreakpoint'
import { DetailPanel } from '../components/DetailPanel'
import { DriveToolbar } from '../components/DriveToolbar'
import { useDrive } from '../hooks/useDrive'
import DriveProvider from '../provider'

export default function DriveLayout() {
    return (
        <DriveProvider>
            <DriveLayoutInner />
        </DriveProvider>
    )
}

function DriveLayoutInner() {
    const { selectedItem, selectItem } = useDrive()
    const isMobile = useBreakpoint() === 'mobile'
    const showDetail = !!selectedItem && !isMobile

    return (
        <YStack flex={1} backgroundColor="$background">
            <DriveToolbar />
            <XStack flex={1}>
                <View style={{ flex: 1 }}>
                    <Slot />
                </View>
                <DetailPanel
                    isVisible={showDetail}
                    item={selectedItem}
                    onClose={() => selectItem(null)}
                />
            </XStack>
        </YStack>
    )
}
