import type { EditorHandle, EditorResult } from '@tinycld/core/lib/editor-types'

interface UseMailEditorOptions {
    initialContent?: string
    placeholder?: string
}

export declare function useMailEditor(options?: UseMailEditorOptions): EditorResult

export declare function setContentWhenReady(editor: EditorHandle, content: string): () => void
