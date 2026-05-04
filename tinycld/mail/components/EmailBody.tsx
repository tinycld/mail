import { pb } from '@tinycld/core/lib/pocketbase'
import { proxyImageUrls } from '@tinycld/core/lib/proxy-image-urls'
import { useCallback, useEffect, useRef, useState } from 'react'
import { Platform, Text, View } from 'react-native'
import { rewriteCidReferences } from './rewrite-cid-references'

interface EmailBodyProps {
    collectionId: string
    recordId: string
    filename: string
    cidMap?: Record<string, string> | null
}

function useEmailHtml(
    collectionId: string,
    recordId: string,
    filename: string,
    cidMap: Record<string, string> | null | undefined
) {
    const [html, setHtml] = useState('')

    useEffect(() => {
        if (!filename) return

        const token = pb.authStore.token
        const url = pb.files.getURL({ collectionId, id: recordId }, filename)
        fetch(url)
            .then((res) => res.text())
            .then((raw) => {
                // Resolve cid: → PB file URL AFTER proxyImageUrls. The proxy
                // wraps remote http(s) <img src> for privacy/auth-token
                // injection but should not re-wrap our own PB-internal
                // attachment URLs — wrapping them sends the request through
                // the image-proxy endpoint which then tries to fetch back
                // from the PB host, breaking under any cross-origin dev
                // setup (Expo on a different port from PB).
                const proxied = proxyImageUrls(raw, token)
                setHtml(rewriteCidReferences(proxied, collectionId, recordId, cidMap))
            })
            .catch(() => setHtml(''))
    }, [collectionId, recordId, filename, cidMap])

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
        style.textContent = `
            html, body { height: auto !important; }
            body {
                font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
                font-size: 14px;
                line-height: 1.5;
                color: #1f2937;
            }
        `
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

export function EmailBody({ collectionId, recordId, filename, cidMap }: EmailBodyProps) {
    const html = useEmailHtml(collectionId, recordId, filename, cidMap)
    const { iframeRef, height, handleLoad } = useIframeAutoHeight(html)

    if (!filename) return null

    if (Platform.OS === 'web') {
        return (
            <View className="p-4 flex-1 rounded-lg bg-white">
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

    // Native: render the stripped HTML as plain text. Do NOT set `flex-1`
    // here — this view sits inside a ScrollView whose contentContainer has
    // `flexGrow: 1`, and `flex-1` on a ScrollView child takes all the
    // available height of the content container. On native that interacts
    // badly with sibling layout (notably the AttachmentStrip's expanding
    // panel) and can collapse the visible body area to zero.
    return (
        <View className="p-4 rounded-lg bg-white">
            <Text className="text-black" style={{ fontSize: 14, lineHeight: 22 }}>
                {html.replace(/<[^>]*>/g, '')}
            </Text>
        </View>
    )
}
