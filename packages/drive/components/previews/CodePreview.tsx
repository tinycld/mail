import { useCallback, useEffect, useState } from 'react'
import { Platform, ScrollView, StyleSheet } from 'react-native'
import { SizableText, Spinner, YStack } from 'tamagui'
import { getFileURL } from '../../lib/file-url'
import type { PreviewProps } from '../../lib/preview-registry'

export function CodePreview({ item }: PreviewProps) {
    const [content, setContent] = useState<string | null>(null)
    const [loading, setLoading] = useState(true)
    const fileUrl = getFileURL(item)

    const loadContent = useCallback(async () => {
        if (!fileUrl || Platform.OS !== 'web') {
            setLoading(false)
            return
        }
        try {
            const resp = await fetch(fileUrl)
            const text = await resp.text()
            setContent(text.slice(0, 100_000))
        } catch {
            setContent('Failed to load file content')
        } finally {
            setLoading(false)
        }
    }, [fileUrl])

    useEffect(() => {
        loadContent()
    }, [loadContent])

    if (loading) {
        return (
            <YStack flex={1} items="center" justify="center">
                <Spinner />
            </YStack>
        )
    }

    if (content === null) {
        return (
            <YStack flex={1} items="center" justify="center" p="$4">
                <SizableText color="$color10">Cannot preview this file</SizableText>
            </YStack>
        )
    }

    return (
        <ScrollView style={styles.container}>
            <SizableText size="$3" color="$color" style={styles.code}>
                {content}
            </SizableText>
        </ScrollView>
    )
}

const styles = StyleSheet.create({
    container: {
        flex: 1,
        padding: 16,
    },
    code: {
        fontFamily: 'monospace',
    },
})
