import {
    BlockquoteBridge,
    BoldBridge,
    BulletListBridge,
    CoreBridge,
    DropCursorBridge,
    type EditorBridge,
    HardBreakBridge,
    HistoryBridge,
    ItalicBridge,
    LinkBridge,
    OrderedListBridge,
    PlaceholderBridge,
    UnderlineBridge,
    useEditorBridge,
} from '@10play/tentap-editor'
import { type RefObject, useMemo } from 'react'
import { Platform } from 'react-native'
import { useThemeColor } from '~/lib/use-app-theme'
import type { RichTextEditorHandle } from '../components/RichTextEditor'

function buildEditorCSS(colors: { bg: string; fg: string; placeholder: string; accent: string }) {
    return `
        * {
            background-color: ${colors.bg};
            color: ${colors.fg};
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;
        }
        .ProseMirror {
            padding: 0;
            min-height: 100%;
            font-size: 14px;
            line-height: 1.5;
        }
        .ProseMirror:focus {
            outline: none;
        }
        .is-editor-empty:first-child::before {
            color: ${colors.placeholder};
            content: attr(data-placeholder);
            float: left;
            height: 0;
            pointer-events: none;
        }
        blockquote {
            border-left: 3px solid ${colors.placeholder};
            padding-left: 1rem;
            margin-left: 0;
        }
        a {
            color: ${colors.accent};
            text-decoration: underline;
        }
        ul, ol {
            padding-left: 1.5rem;
        }
    `
}

const baseBridgeExtensions = [
    BoldBridge,
    ItalicBridge,
    UnderlineBridge,
    BulletListBridge,
    OrderedListBridge,
    BlockquoteBridge,
    LinkBridge,
    HistoryBridge,
    HardBreakBridge,
    DropCursorBridge,
]

interface UseMailEditorOptions {
    initialContent?: string
    placeholder?: string
}

export function useMailEditor(options: UseMailEditorOptions = {}) {
    const bgColor = useThemeColor('background')
    const fgColor = useThemeColor('foreground')
    const placeholderColor = useThemeColor('field-placeholder')
    const accentColor = useThemeColor('accent')
    const placeholder = options.placeholder ?? ''

    const bridgeExtensions = useMemo(() => {
        const css = buildEditorCSS({
            bg: bgColor,
            fg: fgColor,
            placeholder: placeholderColor,
            accent: accentColor,
        })
        return [
            CoreBridge.configureCSS(css),
            ...baseBridgeExtensions,
            PlaceholderBridge.configureExtension({ placeholder }),
        ]
    }, [bgColor, fgColor, placeholderColor, accentColor, placeholder])

    const editorTheme = useMemo(() => ({ webview: { backgroundColor: bgColor } }), [bgColor])

    return useEditorBridge({
        initialContent: options.initialContent,
        bridgeExtensions,
        theme: editorTheme,
    })
}

export function useEditorHandle(editor: EditorBridge, ref: RefObject<RichTextEditorHandle | null>) {
    ref.current = {
        getHTML: () => editor.getHTML(),
        getText: () => editor.getText(),
        focus: () => editor.focus('end'),
        clear: () => editor.setContent(''),
    }
}

export function setContentWhenReady(editor: EditorBridge, content: string): () => void {
    if (Platform.OS === 'web') {
        return setContentOnWeb(editor, content)
    }
    setContentOnNative(editor, content)
    return () => {}
}

function setContentOnWeb(editor: EditorBridge, content: string): () => void {
    const escaped = JSON.stringify(content)
    const js = `(function trySet(n) {
        var pm = document.querySelector('.ProseMirror');
        if (pm && pm.editor) { pm.editor.commands.setContent(${escaped}); }
        else if (n < 30) { setTimeout(function() { trySet(n+1); }, 100); }
    })(0);`

    const tryInject = () => {
        const ref = editor.webviewRef?.current
        if (ref) {
            ref.injectJavaScript(js)
            return true
        }
        return false
    }

    if (tryInject()) return () => {}

    let attempts = 0
    const interval = setInterval(() => {
        attempts++
        if (tryInject() || attempts >= 30) {
            clearInterval(interval)
        }
    }, 100)

    return () => clearInterval(interval)
}

function setContentOnNative(editor: EditorBridge, content: string) {
    const state = editor.getEditorState()
    if (state && 'isReady' in state && state.isReady) {
        editor.setContent(content)
        return
    }
    const unsub = editor._subscribeToEditorStateUpdate(newState => {
        if ('isReady' in newState && newState.isReady) {
            unsub()
            editor.setContent(content)
        }
    })
}
