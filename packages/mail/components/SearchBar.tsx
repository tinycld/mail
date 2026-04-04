import { Menu, Search, SlidersHorizontal } from 'lucide-react-native'
import { Pressable, StyleSheet, TextInput, View } from 'react-native'
import { useTheme } from 'tamagui'

interface SearchBarProps {
    value: string
    onChangeText: (text: string) => void
    onMenuPress?: () => void
}

export function SearchBar({ value, onChangeText, onMenuPress }: SearchBarProps) {
    const theme = useTheme()

    return (
        <View
            style={[
                styles.container,
                {
                    backgroundColor: theme.sidebarBackground.val,
                    borderColor: theme.borderColor.val,
                },
            ]}
        >
            {onMenuPress ? (
                <Pressable onPress={onMenuPress} style={styles.menuButton}>
                    <Menu size={20} color={theme.color8.val} />
                </Pressable>
            ) : null}
            <Search size={18} color={theme.color8.val} />
            <TextInput
                style={[styles.input, { color: theme.color.val }]}
                placeholder="Search mail"
                placeholderTextColor={theme.placeholderColor.val}
                value={value}
                onChangeText={onChangeText}
            />
            <Pressable style={styles.filterButton}>
                <SlidersHorizontal size={18} color={theme.color8.val} />
            </Pressable>
        </View>
    )
}

const styles = StyleSheet.create({
    container: {
        flexDirection: 'row',
        alignItems: 'center',
        height: 44,
        borderRadius: 22,
        paddingHorizontal: 16,
        gap: 10,
        borderWidth: 1,
    },
    input: {
        flex: 1,
        fontSize: 15,
    },
    filterButton: {
        padding: 4,
    },
    menuButton: {
        padding: 2,
    },
})
