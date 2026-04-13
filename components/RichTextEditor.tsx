import { type EditorBridge, RichText } from '@10play/tentap-editor'
import { View } from 'react-native'

export interface RichTextEditorHandle {
    getHTML: () => Promise<string>
    getText: () => Promise<string>
    focus: () => void
    clear: () => void
}

interface RichTextEditorProps {
    editor: EditorBridge
}

export function RichTextEditor({ editor }: RichTextEditorProps) {
    return (
        <View style={{ flex: 1, minHeight: 100 }}>
            <RichText editor={editor} scrollEnabled={false} />
        </View>
    )
}
