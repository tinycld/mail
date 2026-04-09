import { Menu, Search, SlidersHorizontal } from 'lucide-react-native'
import { Pressable, StyleSheet, View } from 'react-native'
import { useTheme } from 'tamagui'
import { PlainInput } from '~/ui/PlainInput'
import type { AdvancedSearchFilters } from '../hooks/useSearchState'
import { AdvancedSearchDropdown } from './AdvancedSearchDropdown'

interface SearchBarProps {
    value: string
    onChangeText: (text: string) => void
    onMenuPress?: () => void
    isFilterOpen: boolean
    onFilterOpenChange: (open: boolean) => void
    onApplyFilters: (filters: AdvancedSearchFilters) => void
    activeFilterCount: number
    currentFilters: AdvancedSearchFilters
}

export function SearchBar({
    value,
    onChangeText,
    onMenuPress,
    isFilterOpen,
    onFilterOpenChange,
    onApplyFilters,
    activeFilterCount,
    currentFilters,
}: SearchBarProps) {
    const theme = useTheme()
    const hasActiveFilters = activeFilterCount > 0
    const filterIconColor = hasActiveFilters ? theme.accentBackground.val : theme.color8.val

    return (
        <View style={styles.wrapper}>
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
                <PlainInput
                    style={[styles.input, { color: theme.color.val }]}
                    placeholder="Search mail"
                    placeholderTextColor={theme.placeholderColor.val}
                    value={value}
                    onChangeText={onChangeText}
                />
                <Pressable
                    style={styles.filterButton}
                    onPress={() => onFilterOpenChange(!isFilterOpen)}
                >
                    <SlidersHorizontal size={18} color={filterIconColor} />
                </Pressable>
            </View>
            {isFilterOpen ? (
                <AdvancedSearchDropdown
                    onApply={onApplyFilters}
                    onClose={() => onFilterOpenChange(false)}
                    initialFilters={currentFilters}
                />
            ) : null}
        </View>
    )
}

const styles = StyleSheet.create({
    wrapper: {
        position: 'relative',
        zIndex: 100,
    },
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
