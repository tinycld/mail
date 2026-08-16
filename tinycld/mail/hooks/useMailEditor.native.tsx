import { useRichEditor } from '@tinycld/core/lib/editor/rich'
import type { EditorResult } from '@tinycld/core/lib/editor/types'

interface UseMailEditorOptions {
    initialContent?: string
    placeholder?: string
    autofocus?: boolean
}

/**
 * Compose-window editor on native: Tiptap in a WebView via TenTap.
 *
 * A thin wrapper over the shared editor in core. Mail's wire format is HTML and
 * always will be, so it stays on `contentFormat: 'html'` and never touches the
 * markdown side.
 */
export function useMailEditor(options: UseMailEditorOptions = {}): EditorResult {
    return useRichEditor({ ...options, contentFormat: 'html' })
}

export { setContentWhenReady } from '@tinycld/core/lib/editor/rich'
