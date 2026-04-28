import { pb } from '@tinycld/core/lib/pocketbase'
import { proxyImageUrls } from '@tinycld/core/lib/proxy-image-urls'
import { useCallback, useEffect, useRef, useState } from 'react'
import { Platform, Text, View } from 'react-native'

interface EmailBodyProps {
    collectionId: string
    recordId: string
    filename: string
}

function useEmailHtml(collectionId: string, recordId: string, filename: string) {
    const [html, setHtml] = useState('')

    useEffect(() => {
        if (!filename) return

        const token = pb.authStore.token
        const url = pb.files.getURL({ collectionId, id: recordId }, filename)
        fetch(url)
            .then((res) => res.text())
            .then((raw) => setHtml(proxyImageUrls(raw, token)))
            .catch(() => setHtml(''))
    }, [collectionId, recordId, filename])

    return html
}

function useIframeAutoHeight(html: string) {
    const iframeRef = useRef<HTMLIFrameElement>(null)
    const [height, setHeight] = useState(300)

    const handleLoad = useCallback(() => {
        const doc = iframeRef.current?.contentDocument
        if (!doc) return

        // Prevent infinite resize loop: without this, setting the iframe
        // height to scrollHeight can cause the content to grow to match
        const style = doc.createElement('style')
        style.textContent = 'html, body { height: auto !important; }'
        doc.head.appendChild(style)

        const updateHeight = () => {
            const scrollH = doc.documentElement.scrollHeight
            const bodyStyle = doc.defaultView?.getComputedStyle(doc.body)
            const marginTop = parseInt(bodyStyle?.marginTop || '0', 10)
            const marginBottom = parseInt(bodyStyle?.marginBottom || '0', 10)
            const h = scrollH + marginTop + marginBottom
            if (h > 0) setHeight(h)
        }

        updateHeight()

        const observer = new ResizeObserver(updateHeight)
        observer.observe(doc.documentElement)
        return () => observer.disconnect()
    }, [])

    // Re-measure when html changes after iframe is already loaded
    // biome-ignore lint/correctness/useExhaustiveDependencies: `html` is the intentional trigger — the effect re-measures the iframe each time the body content changes, but reads the dimensions from the DOM, not from `html` directly
    useEffect(() => {
        const doc = iframeRef.current?.contentDocument
        if (doc?.documentElement) {
            const h = doc.documentElement.scrollHeight
            if (h > 0) setHeight(h)
        }
    }, [html])

    return { iframeRef, height, handleLoad }
}

export function EmailBody({ collectionId, recordId, filename }: EmailBodyProps) {
    const html = useEmailHtml(collectionId, recordId, filename)
    const { iframeRef, height, handleLoad } = useIframeAutoHeight(html)

    if (!filename) return null

    if (Platform.OS === 'web') {
        return (
            <View className="p-4 flex-1 rounded-lg" style={{ backgroundColor: '#fff' }}>
                <iframe
                    ref={iframeRef}
                    sandbox="allow-same-origin"
                    srcDoc={html}
                    onLoad={handleLoad}
                    style={{
                        border: 'none',
                        width: '100%',
                        height,
                        colorScheme: 'light',
                    }}
                    title="Email body"
                />
            </View>
        )
    }

    return (
        <View className="p-4 flex-1 rounded-lg" style={{ backgroundColor: '#fff' }}>
            <Text style={{ fontSize: 14, lineHeight: 22, color: '#000' }}>{html.replace(/<[^>]*>/g, '')}</Text>
        </View>
    )
}
