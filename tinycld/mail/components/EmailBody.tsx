import { useFileToken } from '@tinycld/core/file-viewer/use-authed-file-url'
import { captureException } from '@tinycld/core/lib/errors'
import { pb } from '@tinycld/core/lib/pocketbase'
import { proxyImageUrls } from '@tinycld/core/lib/proxy-image-urls'
import { serverFetch } from '@tinycld/core/lib/server-fetch'
import { useCallback, useEffect, useRef, useState } from 'react'
import { Platform, Text, View } from 'react-native'
import { WebView, type WebViewMessageEvent } from 'react-native-webview'
import { measureContentHeight } from '../lib/email-frame-height'
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
    const [failed, setFailed] = useState(false)
    // mail_messages files sit behind the record's viewRule (mailbox
    // membership). The SDK carries auth on its own requests, but this is a
    // raw fetch outside that pipeline, so the URL needs an explicit
    // ?token= — exactly what core's file viewer does for the same reason.
    const { data: fileToken } = useFileToken()

    useEffect(() => {
        if (!filename || !fileToken) return

        const token = pb.authStore.token
        const url = pb.files.getURL({ collectionId, id: recordId }, filename, {
            token: fileToken,
        })
        setFailed(false)
        serverFetch(url)
            .then(res => {
                if (!res.ok) throw new Error(`body fetch: HTTP ${res.status}`)
                return res.text()
            })
            .then(raw => {
                // Resolve cid: → PB file URL AFTER proxyImageUrls. The proxy
                // wraps remote http(s) <img src> for privacy/auth-token
                // injection but should not re-wrap our own PB-internal
                // attachment URLs — wrapping them sends the request through
                // the image-proxy endpoint which then tries to fetch back
                // from the PB host, breaking under any cross-origin dev
                // setup (Expo on a different port from PB).
                const proxied = proxyImageUrls(raw, token)
                setHtml(rewriteCidReferences(proxied, collectionId, recordId, cidMap, fileToken))
            })
            .catch((err: unknown) => {
                // A failed fetch must not render as an EMPTY email — the user
                // has no way to tell "blank message" from "we couldn't load
                // it", and the failure was invisible to Sentry too.
                captureException('mail.body.fetch', err, { recordId, filename })
                setHtml('')
                setFailed(true)
            })
    }, [collectionId, recordId, filename, cidMap, fileToken])

    return { html, failed }
}

function BodyLoadFailed() {
    return (
        <View className="p-4 rounded-lg bg-surface-secondary">
            <Text className="text-[13px] text-danger">
                Couldn't load this message. Check your connection and reopen the thread to retry.
            </Text>
        </View>
    )
}

function useIframeAutoHeight() {
    const iframeRef = useRef<HTMLIFrameElement>(null)
    const observerRef = useRef<ResizeObserver | null>(null)
    const [height, setHeight] = useState(300)

    // srcDoc loads once empty and again with the fetched body, and onLoad
    // ignores a returned cleanup, so the observer lives here: replaced on each
    // load, disconnected on unmount.
    useEffect(() => () => observerRef.current?.disconnect(), [])

    const handleLoad = useCallback(() => {
        const doc = iframeRef.current?.contentDocument
        if (!doc) return

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
            const h = measureContentHeight(doc)
            if (h > 0) setHeight(h)
        }
        updateHeight()

        observerRef.current?.disconnect()
        observerRef.current = new ResizeObserver(updateHeight)
        observerRef.current.observe(doc.documentElement)
    }, [])

    return { iframeRef, height, handleLoad }
}

export function EmailBody({ collectionId, recordId, filename, cidMap }: EmailBodyProps) {
    const { html, failed } = useEmailHtml(collectionId, recordId, filename, cidMap)
    const { iframeRef, height, handleLoad } = useIframeAutoHeight()

    if (!filename) return null
    if (failed) return <BodyLoadFailed />

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

    return <NativeHtmlBody html={html} />
}

const HEIGHT_REPORTER_SCRIPT = `
(function() {
    function report() {
        // The <html> box, not documentElement.scrollHeight: scrollHeight
        // never drops below the WebView's own height, so a body that shrinks
        // would keep its old height (see measureContentHeight).
        var h = Math.ceil(document.documentElement.getBoundingClientRect().height);
        window.ReactNativeWebView.postMessage(String(h));
    }
    report();
    window.addEventListener('load', report);
    new ResizeObserver(report).observe(document.documentElement);
})();
true;
`

function wrapHtml(body: string): string {
    return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><style>html,body{margin:0;padding:0;height:auto}body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif;font-size:14px;line-height:1.5;color:#1f2937;padding:16px}img{max-width:100%;height:auto}</style></head><body>${body}</body></html>`
}

function NativeHtmlBody({ html }: { html: string }) {
    // Mirror web's iframe pattern: render the email HTML inside a WebView and
    // let the document report its height back so the surrounding ScrollView
    // (the thread list) handles scrolling, not a nested WebView. Without
    // this, every email becomes its own scrollable region nested in the
    // outer scroll — confusing on touch devices.
    const [height, setHeight] = useState(120)
    const onMessage = useCallback((event: WebViewMessageEvent) => {
        const reported = Number(event.nativeEvent.data)
        if (Number.isFinite(reported) && reported > 0) setHeight(reported)
    }, [])

    if (!html) return <View className="p-4 rounded-lg bg-white" style={{ minHeight: 120 }} />

    return (
        <View className="rounded-lg overflow-hidden bg-white" style={{ height }}>
            <WebView
                originWhitelist={['*']}
                source={{ html: wrapHtml(html) }}
                onMessage={onMessage}
                injectedJavaScript={HEIGHT_REPORTER_SCRIPT}
                javaScriptEnabled
                domStorageEnabled={false}
                scrollEnabled={false}
                showsVerticalScrollIndicator={false}
                style={{ backgroundColor: 'white' }}
            />
        </View>
    )
}
