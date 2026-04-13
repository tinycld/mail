import { useThemeColor } from 'heroui-native'
import { Menu, Search, SlidersHorizontal } from 'lucide-react-native'
import { Pressable, View } from 'react-native'
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
    const [
        foregroundColor,
        mutedColor,
        accentColor,
        borderColor,
        sidebarBgColor,
        placeholderColor,
    ] = useThemeColor([
        'foreground',
        'muted',
        'accent',
        'border',
        'surface-secondary',
        'field-placeholder',
    ])
    const hasActiveFilters = activeFilterCount > 0
    const filterIconColor = hasActiveFilters ? accentColor : mutedColor

    return (
        <View style={{ position: 'relative', zIndex: 100 }}>
            <View
                style={{
                    flexDirection: 'row',
                    alignItems: 'center',
                    height: 44,
                    borderRadius: 22,
                    paddingHorizontal: 16,
                    gap: 10,
                    borderWidth: 1,
                    backgroundColor: sidebarBgColor,
                    borderColor,
                }}
            >
                {onMenuPress ? (
                    <Pressable onPress={onMenuPress} style={{ padding: 2 }}>
                        <Menu size={20} color={mutedColor} />
                    </Pressable>
                ) : null}
                <Search size={18} color={mutedColor} />
                <PlainInput
                    style={{ flex: 1, fontSize: 15, color: foregroundColor }}
                    placeholder="Search mail"
                    placeholderTextColor={placeholderColor}
                    value={value}
                    onChangeText={onChangeText}
                />
                <Pressable style={{ padding: 4 }} onPress={() => onFilterOpenChange(!isFilterOpen)}>
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
