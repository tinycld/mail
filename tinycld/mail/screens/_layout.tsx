import { useBreakpoint } from '@tinycld/core/components/workspace/useBreakpoint'
import { useWorkspaceLayout } from '@tinycld/core/components/workspace/useWorkspaceLayout'
import { Slot } from 'expo-router'
import { useEffect, useMemo, useState } from 'react'
import { View } from 'react-native'
import { ComposeWindow } from '../components/ComposeWindow'
import { SearchBar } from '../components/SearchBar'
import { composeEvents } from '../hooks/composeEvents'
import { useMailSearch } from '../hooks/useMailSearch'
import {
    type AdvancedSearchFilters,
    countActiveFilters,
    hasActiveFilters,
    SearchContext,
} from '../hooks/useSearchState'
import { useComposeStore } from '../stores/compose-store'

export { useThreadListContext } from '../stores/thread-list-store'

export default function MailLayout() {
    const [searchQuery, setSearchQuery] = useState('')
    const [advancedFilters, setAdvancedFilters] = useState<AdvancedSearchFilters>({})
    const [isFilterOpen, setIsFilterOpen] = useState(false)
    const breakpoint = useBreakpoint()
    const { setDrawerOpen } = useWorkspaceLayout()
    const composeMode = useComposeStore(s => s.mode)

    useEffect(() => {
        return composeEvents.subscribe(() => {
            const { mode, open } = useComposeStore.getState()
            if (mode === 'closed') {
                open()
            }
        })
    }, [])

    const { results, total, isSearching } = useMailSearch(searchQuery, advancedFilters)

    const activeFilterCount = countActiveFilters(advancedFilters)

    const searchValue = useMemo(
        () => ({
            query: searchQuery,
            results,
            total,
            isSearching,
            isActive: searchQuery.length >= 2 || hasActiveFilters(advancedFilters),
            filters: advancedFilters,
        }),
        [searchQuery, results, total, isSearching, advancedFilters]
    )

    const isComposeVisible = composeMode !== 'closed' && composeMode !== 'inline'
    const isMobile = breakpoint === 'mobile'

    return (
        <SearchContext.Provider value={searchValue}>
            <View className="flex-1 bg-background">
                <View className="px-4 py-2">
                    <SearchBar
                        value={searchQuery}
                        onChangeText={setSearchQuery}
                        onMenuPress={isMobile ? () => setDrawerOpen(true) : undefined}
                        isFilterOpen={isFilterOpen}
                        onFilterOpenChange={setIsFilterOpen}
                        onApplyFilters={setAdvancedFilters}
                        activeFilterCount={activeFilterCount}
                        currentFilters={advancedFilters}
                    />
                </View>
                <View className="flex-1">
                    <Slot />
                </View>
                <ComposeWindow isVisible={isComposeVisible} />
            </View>
        </SearchContext.Provider>
    )
}
