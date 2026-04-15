import { useEffect, useState } from 'react'
import { Platform, Text, View } from 'react-native'
import { pb } from '~/lib/pocketbase'
import { proxyImageUrls } from '~/lib/proxy-image-urls'

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
            .then(raw => setHtml(proxyImageUrls(raw)))
            .catch(() => setHtml(''))
    }, [collectionId, recordId, filename])

    return html
}

export function EmailBody({ collectionId, recordId, filename }: EmailBodyProps) {
    const html = useEmailHtml(collectionId, recordId, filename)

    if (!filename) return null

    if (Platform.OS === 'web') {
        return (
            <View className="p-4 flex-1 rounded-lg" style={{ backgroundColor: '#fff' }}>
                <iframe
                    sandbox=""
                    srcDoc={html}
                    style={{
                        border: 'none',
                        width: '100%',
                        minHeight: 300,
                        flex: 1,
                        colorScheme: 'light',
                    }}
                    title="Email body"
                />
            </View>
        )
    }

    return (
        <View className="p-4 flex-1 rounded-lg" style={{ backgroundColor: '#fff' }}>
            <Text style={{ fontSize: 14, lineHeight: 22, color: '#000' }}>
                {html.replace(/<[^>]*>/g, '')}
            </Text>
        </View>
    )
}
