import { Menu } from '@tamagui/menu'
import { Check, MoreVertical } from 'lucide-react-native'
import { Pressable, View as RNView, StyleSheet } from 'react-native'
import { Separator, useTheme, View } from 'tamagui'
import type { CalendarColorKey } from '../types'
import { CALENDAR_COLOR_GRID, getCalendarColorResolved } from './calendar-colors'

interface CalendarMenuProps {
    currentColor: CalendarColorKey
    onColorChange: (color: CalendarColorKey) => void
    onShowOnly: () => void
}

export function CalendarMenu({ currentColor, onColorChange, onShowOnly }: CalendarMenuProps) {
    const theme = useTheme()

    return (
        <Menu>
            <Menu.Trigger asChild>
                <View>
                    <Pressable style={styles.triggerButton} hitSlop={8}>
                        <MoreVertical size={14} color={theme.color8.val} />
                    </Pressable>
                </View>
            </Menu.Trigger>

            <Menu.Portal zIndex={100}>
                <Menu.Content
                    borderRadius={8}
                    minWidth={220}
                    backgroundColor="$background"
                    borderColor="$borderColor"
                    borderWidth={1}
                    paddingVertical="$1"
                    shadowColor="#000"
                    shadowOffset={{ width: 0, height: 4 }}
                    shadowOpacity={0.15}
                    shadowRadius={12}
                >
                    <Menu.Item onSelect={onShowOnly} gap="$2">
                        <Menu.ItemTitle size="$3">Display this only</Menu.ItemTitle>
                    </Menu.Item>

                    <Separator marginVertical="$1" />

                    <View paddingHorizontal="$3" paddingVertical="$2" gap={6}>
                        {CALENDAR_COLOR_GRID.map(row => (
                            <View key={row.join('-')} flexDirection="row" gap={6}>
                                {row.map(colorKey => {
                                    const { bg } = getCalendarColorResolved(colorKey)
                                    return (
                                        <Menu.Item
                                            key={colorKey}
                                            onSelect={() => onColorChange(colorKey)}
                                            padding={2}
                                            borderRadius={14}
                                            minWidth="auto"
                                            width={28}
                                            height={28}
                                            alignItems="center"
                                            justifyContent="center"
                                        >
                                            <RNView
                                                style={[styles.colorDot, { backgroundColor: bg }]}
                                            >
                                                {currentColor === colorKey && (
                                                    <Check size={12} color="#ffffff" />
                                                )}
                                            </RNView>
                                        </Menu.Item>
                                    )
                                })}
                            </View>
                        ))}
                    </View>
                </Menu.Content>
            </Menu.Portal>
        </Menu>
    )
}

const styles = StyleSheet.create({
    triggerButton: {
        padding: 4,
        borderRadius: 4,
    },
    colorDot: {
        width: 24,
        height: 24,
        borderRadius: 12,
        alignItems: 'center',
        justifyContent: 'center',
    },
})
