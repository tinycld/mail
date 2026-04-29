import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { type Shortcut, useRegisterShortcuts, useShortcutScope } from '@tinycld/core/lib/shortcuts'
import type { useRouter } from 'expo-router'
import { useEffect, useMemo, useRef } from 'react'
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
    const storedIndex = useThreadListStore((s) => s.focusedIndex)
    const hasFocus = useThreadListStore((s) => s.hasFocus)
    const setFocusedIndex = useThreadListStore((s) => s.setFocusedIndex)
    const clearFocus = useThreadListStore((s) => s.clearFocus)
    const orgHref = useOrgHref()

    useShortcutScope('list')

    // Reset focus when the folder/label scope changes so we don't carry over
    // a stale position. Done in an effect so the store update doesn't fire
    // during render.
    const prevFolderRef = useRef(folder)
    const prevLabelsRef = useRef(labels.join(','))
    const labelsKey = labels.join(',')
    useEffect(() => {
        if (folder !== prevFolderRef.current || labelsKey !== prevLabelsRef.current) {
            prevFolderRef.current = folder
            prevLabelsRef.current = labelsKey
            clearFocus()
        }
    }, [folder, labelsKey, clearFocus])

    // Clamp the persisted index to the current list so we don't highlight a
    // nonexistent row after the list shrinks.
    const focusedIndex = items.length === 0 ? 0 : Math.min(storedIndex, items.length - 1)
    // Only expose ids when the user has affirmatively engaged keyboard nav;
    // otherwise rows render without the focus indicator and Enter is a no-op.
    const focusedId = hasFocus ? (items[focusedIndex]?.stateId ?? null) : null
    const focusedThreadId = hasFocus ? (items[focusedIndex]?.threadId ?? null) : null

    const shortcuts = useMemo<Shortcut[]>(() => {
        if (!isEnabled) return []
        const openFocused = () => {
            if (!focusedThreadId) return
            router.push(orgHref('mail/[id]', { id: focusedThreadId }))
        }
        const lastIndex = Math.max(items.length - 1, 0)
        // First j/k from no-focus lands on row 0 instead of advancing past it.
        const next = () =>
            hasFocus ? setFocusedIndex((i) => Math.min(i + 1, lastIndex)) : setFocusedIndex(0)
        const prev = () => (hasFocus ? setFocusedIndex((i) => Math.max(i - 1, 0)) : setFocusedIndex(0))
        return [
            {
                id: 'mail.list.next',
                keys: 'j',
                scope: 'list',
                group: 'Mail',
                description: 'Next conversation',
                run: next,
            },
            {
                id: 'mail.list.prev',
                keys: 'k',
                scope: 'list',
                group: 'Mail',
                description: 'Previous conversation',
                run: prev,
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
    }, [isEnabled, items.length, hasFocus, focusedId, focusedThreadId, orgHref, router, setFocusedIndex, toggleSelect])

    useRegisterShortcuts(shortcuts)

    return { focusedIndex, focusedId }
}
