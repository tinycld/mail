import type {
    EditorCommands,
    EditorHandle,
    EditorResult,
    EditorToolbarState,
} from '@tinycld/core/lib/editor/types'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import Placeholder from '@tiptap/extension-placeholder'
import { EditorContent, useEditor } from '@tiptap/react'
import StarterKit from '@tiptap/starter-kit'
import { useMemo } from 'react'
import { View } from 'react-native'
import '../styles/editor.css'

interface UseMailEditorOptions {
    initialContent?: string
    placeholder?: string
    autofocus?: boolean
}

export function useMailEditor(options: UseMailEditorOptions = {}): EditorResult {
    const placeholderColor = useThemeColor('field-placeholder')
    const primaryColor = useThemeColor('primary')

    const tiptapEditor = useEditor({
        extensions: [
            StarterKit.configure({ link: { openOnClick: false } }),
            Placeholder.configure({ placeholder: options.placeholder ?? '' }),
        ],
        content: options.initialContent ?? '',
    })

    const editor: EditorHandle = useMemo(() => {
        // tiptap nulls commandManager on destroy, and useEditor can briefly
        // hand back a destroyed instance during remount. Touching `.commands`
        // / `.chain()` in that window throws "Cannot read properties of null
        // (reading 'commands')". Gate every imperative call on a live editor.
        const isLive = () => !!tiptapEditor && !tiptapEditor.isDestroyed
        return {
            getHTML: () => Promise.resolve(isLive() ? (tiptapEditor?.getHTML() ?? '') : ''),
            getText: () => Promise.resolve(isLive() ? (tiptapEditor?.getText() ?? '') : ''),
            setContent: (html: string) => {
                if (!isLive()) return
                tiptapEditor?.commands.setContent(html)
            },
            focus: (position?: 'start' | 'end') => {
                if (!isLive()) return
                if (position === 'start') {
                    tiptapEditor?.chain().focus('start').run()
                } else {
                    tiptapEditor?.chain().focus('end').run()
                }
            },
            clear: () => {
                if (!isLive()) return
                tiptapEditor?.commands.clearContent()
            },
            getSelection: () => {
                if (!isLive()) return Promise.resolve(null)
                const selection = tiptapEditor?.state.selection
                if (!selection) return Promise.resolve(null)
                return Promise.resolve({ from: selection.from, to: selection.to })
            },
        }
    }, [tiptapEditor])

    const commands: EditorCommands = useMemo(() => {
        const chain = () =>
            tiptapEditor && !tiptapEditor.isDestroyed ? tiptapEditor.chain().focus() : null
        return {
            toggleBold: () => chain()?.toggleBold().run(),
            toggleItalic: () => chain()?.toggleItalic().run(),
            toggleUnderline: () => chain()?.toggleUnderline().run(),
            toggleBulletList: () => chain()?.toggleBulletList().run(),
            toggleOrderedList: () => chain()?.toggleOrderedList().run(),
            toggleBlockquote: () => chain()?.toggleBlockquote().run(),
            toggleHeading: (level: number) =>
                chain()
                    ?.toggleHeading({ level: level as 1 | 2 | 3 | 4 | 5 | 6 })
                    .run(),
            setLink: (url: string) => chain()?.setLink({ href: url }).run(),
            removeLink: () => chain()?.unsetLink().run(),
            undo: () => chain()?.undo().run(),
            redo: () => chain()?.redo().run(),
        }
    }, [tiptapEditor])

    const live = tiptapEditor && !tiptapEditor.isDestroyed ? tiptapEditor : null
    const toolbarState: EditorToolbarState = {
        isBoldActive: live?.isActive('bold') ?? false,
        isItalicActive: live?.isActive('italic') ?? false,
        isUnderlineActive: live?.isActive('underline') ?? false,
        isBulletListActive: live?.isActive('bulletList') ?? false,
        isOrderedListActive: live?.isActive('orderedList') ?? false,
        isBlockquoteActive: live?.isActive('blockquote') ?? false,
        isLinkActive: live?.isActive('link') ?? false,
        currentLink: (live?.getAttributes('link')?.href as string) ?? null,
    }

    const EditorComponent = useMemo(
        () =>
            function MailEditorContent() {
                return (
                    <View
                        className="flex-1 min-h-[100px] tinycld-mail-editor"
                        style={{
                            // @ts-expect-error CSS custom properties for web
                            '--editor-placeholder-color': placeholderColor,
                            '--editor-primary-color': primaryColor,
                        }}
                    >
                        <EditorContent editor={tiptapEditor} />
                    </View>
                )
            },
        [tiptapEditor, placeholderColor, primaryColor]
    )

    return { editor, EditorComponent, commands, toolbarState }
}

export function setContentWhenReady(editor: EditorHandle, content: string): () => void {
    editor.setContent(content)
    return () => {}
}
