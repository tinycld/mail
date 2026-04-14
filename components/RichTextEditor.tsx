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
        <View className="flex-1 min-h-[100px]">
            <RichText editor={editor} scrollEnabled={false} />
        </View>
    )
}
