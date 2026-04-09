import { useEffect, useState } from 'react'
import { Platform, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import { pb } from '~/lib/pocketbase'

interface EmailBodyProps {
    collectionId: string
    recordId: string
    filename: string
}

function useEmailHtml(collectionId: string, recordId: string, filename: string) {
    const [html, setHtml] = useState('')

    useEffect(() => {
        if (!filename) return

        const url = pb.files.getURL({ collectionId, id: recordId }, filename)
        fetch(url)
            .then(res => res.text())
            .then(setHtml)
            .catch(() => setHtml(''))
    }, [collectionId, recordId, filename])

    return html
}

export function EmailBody({ collectionId, recordId, filename }: EmailBodyProps) {
    const theme = useTheme()
    const html = useEmailHtml(collectionId, recordId, filename)

    if (!filename) return null

    if (Platform.OS === 'web') {
        return (
            <View style={styles.container}>
                <iframe
                    sandbox=""
                    srcDoc={html}
                    style={{
                        border: 'none',
                        width: '100%',
                        minHeight: 300,
                        flex: 1,
                        colorScheme: 'auto',
                    }}
                    title="Email body"
                />
            </View>
        )
    }

    return (
        <View style={styles.container}>
            <Text style={[styles.fallback, { color: theme.color.val }]}>
                {html.replace(/<[^>]*>/g, '')}
            </Text>
        </View>
    )
}

const styles = StyleSheet.create({
    container: {
        padding: 16,
        flex: 1,
    },
    fallback: {
        fontSize: 14,
        lineHeight: 22,
    },
})
