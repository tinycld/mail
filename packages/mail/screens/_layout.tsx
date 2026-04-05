import { Slot } from 'one'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { YStack } from 'tamagui'
import { useBreakpoint } from '~/components/workspace/useBreakpoint'
import { useWorkspaceLayout } from '~/components/workspace/useWorkspaceLayout'
import { ComposeWindow } from '../components/ComposeWindow'
import { SearchBar } from '../components/SearchBar'
import { composeEvents } from '../hooks/composeEvents'
import { ComposeContext, type ComposeMode, type ReplyContext } from '../hooks/useComposeState'

export default function MailLayout() {
    const [composeMode, setComposeMode] = useState<ComposeMode>('closed')
    const [replyContext, setReplyContext] = useState<ReplyContext | null>(null)
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
    }, [])

    const openReply = useCallback((context: ReplyContext) => {
        setReplyContext(context)
        setComposeMode('inline')
    }, [])

    const composeValue = useMemo(
        () => ({ mode: composeMode, replyContext, open, minimize, maximize, close, openReply }),
        [composeMode, replyContext, open, minimize, maximize, close, openReply]
    )

    const isComposeVisible = composeMode !== 'closed' && composeMode !== 'inline'
    const isMobile = breakpoint === 'mobile'

    return (
        <ComposeContext.Provider value={composeValue}>
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
        </ComposeContext.Provider>
    )
}
