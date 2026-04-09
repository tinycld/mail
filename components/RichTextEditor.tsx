import { type EditorBridge, RichText } from '@10play/tentap-editor'
import { StyleSheet, View } from 'react-native'

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
        <View style={styles.container}>
            <RichText editor={editor} scrollEnabled={false} />
        </View>
    )
}

const styles = StyleSheet.create({
    container: {
        flex: 1,
        minHeight: 100,
    },
})
