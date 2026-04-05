import { Slot } from 'one'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { YStack } from 'tamagui'
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
import { SearchContext } from '../hooks/useSearchState'

export default function MailLayout() {
    const [composeMode, setComposeMode] = useState<ComposeMode>('closed')
    const [replyContext, setReplyContext] = useState<ReplyContext | null>(null)
    const [draftContext, setDraftContext] = useState<DraftContext | null>(null)
    const [searchQuery, setSearchQuery] = useState('')
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

    const { results, total, isSearching } = useMailSearch(searchQuery)

    const searchValue = useMemo(
        () => ({
            query: searchQuery,
            results,
            total,
            isSearching,
            isActive: searchQuery.length >= 2,
        }),
        [searchQuery, results, total, isSearching]
    )

    const isComposeVisible = composeMode !== 'closed' && composeMode !== 'inline'
    const isMobile = breakpoint === 'mobile'

    return (
        <ComposeContext.Provider value={composeValue}>
            <SearchContext.Provider value={searchValue}>
                <YStack flex={1} backgroundColor="$background">
                    <YStack paddingHorizontal="$4" paddingVertical="$2">
                        <SearchBar
                            value={searchQuery}
                            onChangeText={setSearchQuery}
                            onMenuPress={isMobile ? () => setDrawerOpen(true) : undefined}
                        />
                    </YStack>
                    <YStack flex={1}>
                        <Slot />
                    </YStack>
                    <ComposeWindow isVisible={isComposeVisible} />
                </YStack>
            </SearchContext.Provider>
        </ComposeContext.Provider>
    )
}
