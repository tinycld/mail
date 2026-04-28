import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { type Shortcut, useRegisterShortcuts, useShortcutScope } from '@tinycld/core/lib/shortcuts'
import type { useRouter } from 'expo-router'
import { useMemo, useRef } from 'react'
import type { ThreadListItem } from '../components/thread-list-item'
import { useThreadListStore } from '../stores/thread-list-store'
import { composeEvents } from './composeEvents'

interface UseMailListShortcutsArgs {
    items: ThreadListItem[]
    router: ReturnType<typeof useRouter>
    toggleSelect: (stateId: string) => void
    isEnabled: boolean
    folder: string | null
    labels: string[]
}

export function useMailListShortcuts({
    items,
    router,
    toggleSelect,
    isEnabled,
    folder,
    labels,
}: UseMailListShortcutsArgs) {
    const storedIndex = useThreadListStore(s => s.focusedIndex)
    const setFocusedIndex = useThreadListStore(s => s.setFocusedIndex)
    const orgHref = useOrgHref()

    useShortcutScope('list')

    // Reset the persisted focus when the folder/label scope changes so we
    // don't land on a stale position from the previous view. Compared during
    // render rather than in useEffect (matches the pattern in useMailSelection).
    const prevFolderRef = useRef(folder)
    const prevLabelsRef = useRef(labels.join(','))
    const labelsKey = labels.join(',')
    if (folder !== prevFolderRef.current || labelsKey !== prevLabelsRef.current) {
        prevFolderRef.current = folder
        prevLabelsRef.current = labelsKey
        if (storedIndex !== 0) setFocusedIndex(0)
    }

    // Clamp the persisted index to the current list so we don't highlight a
    // nonexistent row after the list shrinks.
    const focusedIndex = items.length === 0 ? 0 : Math.min(storedIndex, items.length - 1)
    const focusedId = items[focusedIndex]?.stateId ?? null
    const focusedThreadId = items[focusedIndex]?.threadId ?? null

    const shortcuts = useMemo<Shortcut[]>(() => {
        if (!isEnabled) return []
        const openFocused = () => {
            if (!focusedThreadId) return
            router.push(orgHref('mail/[id]', { id: focusedThreadId }))
        }
        return [
            {
                id: 'mail.list.next',
                keys: 'j',
                scope: 'list',
                group: 'Mail',
                description: 'Next conversation',
                run: () => setFocusedIndex(i => Math.min(i + 1, Math.max(items.length - 1, 0))),
            },
            {
                id: 'mail.list.prev',
                keys: 'k',
                scope: 'list',
                group: 'Mail',
                description: 'Previous conversation',
                run: () => setFocusedIndex(i => Math.max(i - 1, 0)),
            },
            {
                id: 'mail.list.open',
                keys: 'Enter',
                scope: 'list',
                group: 'Mail',
                description: 'Open conversation',
                run: openFocused,
            },
            {
                id: 'mail.list.openO',
                keys: 'o',
                scope: 'list',
                group: 'Mail',
                description: 'Open conversation (alt)',
                run: openFocused,
            },
            {
                id: 'mail.list.select',
                keys: 'x',
                scope: 'list',
                group: 'Mail',
                description: 'Toggle selection',
                run: () => {
                    if (!focusedId) return
                    toggleSelect(focusedId)
                },
            },
            {
                id: 'mail.list.compose',
                keys: 'c',
                scope: 'list',
                group: 'Mail',
                description: 'Compose new email',
                run: () => composeEvents.emit(),
            },
        ]
    }, [
        isEnabled,
        items.length,
        focusedId,
        focusedThreadId,
        orgHref,
        router,
        setFocusedIndex,
        toggleSelect,
    ])

    useRegisterShortcuts(shortcuts)

    return { focusedIndex, focusedId }
}
