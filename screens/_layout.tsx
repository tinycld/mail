import { Slot } from 'expo-router'
import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import { View } from 'react-native'
import { useBreakpoint } from '~/components/workspace/useBreakpoint'
import { useWorkspaceLayout } from '~/components/workspace/useWorkspaceLayout'
import { ComposeWindow } from '../components/ComposeWindow'
import { SearchBar } from '../components/SearchBar'
import { composeEvents } from '../hooks/composeEvents'
import {
    ComposeContext,
    type ComposeMode,
    type DraftContext,
    type ReplyContext,
} from '../hooks/useComposeState'
import { useMailSearch } from '../hooks/useMailSearch'
import {
    type AdvancedSearchFilters,
    countActiveFilters,
    hasActiveFilters,
    SearchContext,
} from '../hooks/useSearchState'

interface ThreadListContextValue {
    threadIds: string[]
    setThreadIds: (ids: string[]) => void
}

const ThreadListContext = createContext<ThreadListContextValue>({
    threadIds: [],
    setThreadIds: () => {},
})

export function useThreadListContext() {
    return useContext(ThreadListContext)
}

export default function MailLayout() {
    const [composeMode, setComposeMode] = useState<ComposeMode>('closed')
    const [replyContext, setReplyContext] = useState<ReplyContext | null>(null)
    const [draftContext, setDraftContext] = useState<DraftContext | null>(null)
    const [threadIds, setThreadIds] = useState<string[]>([])
    const threadListValue = useMemo(() => ({ threadIds, setThreadIds }), [threadIds])
    const [searchQuery, setSearchQuery] = useState('')
    const [advancedFilters, setAdvancedFilters] = useState<AdvancedSearchFilters>({})
    const [isFilterOpen, setIsFilterOpen] = useState(false)
    const breakpoint = useBreakpoint()
    const { setDrawerOpen } = useWorkspaceLayout()

    useEffect(() => {
        return composeEvents.subscribe(() => {
            setComposeMode(prev => (prev === 'closed' ? 'open' : prev))
        })
    }, [])

    const open = useCallback(() => setComposeMode('open'), [])
    const minimize = useCallback(() => setComposeMode('minimized'), [])
    const maximize = useCallback(() => setComposeMode('maximized'), [])
    const close = useCallback(() => {
        setComposeMode('closed')
        setReplyContext(null)
        setDraftContext(null)
    }, [])

    const openReply = useCallback((context: ReplyContext) => {
        setReplyContext(context)
        setComposeMode('inline')
    }, [])

    const openDraft = useCallback((context: DraftContext) => {
        setDraftContext(context)
        setReplyContext(null)
        setComposeMode('open')
    }, [])

    const composeValue = useMemo(
        () => ({
            mode: composeMode,
            replyContext,
            draftContext,
            open,
            minimize,
            maximize,
            close,
            openReply,
            openDraft,
        }),
        [
            composeMode,
            replyContext,
            draftContext,
            open,
            minimize,
            maximize,
            close,
            openReply,
            openDraft,
        ]
    )

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
        <ComposeContext.Provider value={composeValue}>
            <SearchContext.Provider value={searchValue}>
                <ThreadListContext.Provider value={threadListValue}>
                    <View className="flex-1 bg-background">
                        <View style={{ paddingHorizontal: 16, paddingVertical: 8 }}>
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
                        <View style={{ flex: 1 }}>
                            <Slot />
                        </View>
                        <ComposeWindow isVisible={isComposeVisible} />
                    </View>
                </ThreadListContext.Provider>
            </SearchContext.Provider>
        </ComposeContext.Provider>
    )
}
