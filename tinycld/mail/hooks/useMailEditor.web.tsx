import { useRichEditor } from '@tinycld/core/lib/editor/rich'
import type { EditorResult } from '@tinycld/core/lib/editor/types'
import '../styles/editor.css'

interface UseMailEditorOptions {
    initialContent?: string
    placeholder?: string
    autofocus?: boolean
}

/**
 * Compose-window editor.
 *
 * A thin wrapper over the shared editor in core. Mail's wire format is HTML and
 * always will be — a message body is HTML on the way out and on the way back
 * from a draft — so it stays on `contentFormat: 'html'` and never touches the
 * markdown side.
 */
export function useMailEditor(options: UseMailEditorOptions = {}): EditorResult {
    return useRichEditor({
        ...options,
        contentFormat: 'html',
        containerClassName: 'flex-1 min-h-[100px] tinycld-mail-editor',
    })
}

export { setContentWhenReady } from '@tinycld/core/lib/editor/rich'
