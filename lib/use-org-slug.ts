import { useEffect, useState } from 'react'
import { Platform } from 'react-native'
import { loadPrimaryOrgFromStorage } from '~/lib/auth'
import { parseSubdomain } from '~/lib/hostname'

// Web: compute once at module load — hostname never changes during a session
const WEB_ORG_SLUG: string =
    Platform.OS === 'web' && typeof window !== 'undefined'
        ? parseSubdomain(window.location.hostname).slug
        : ''

// Native: single shared AsyncStorage read, cached after first resolution
let cachedNativeSlug: string | null = null
let pendingRead: Promise<string | null> | null = null

function readNativeSlug(): Promise<string | null> {
    if (!pendingRead) {
        pendingRead = loadPrimaryOrgFromStorage().then(slug => {
            cachedNativeSlug = slug
            return slug
        })
    }
    return pendingRead
}

export function useOrgSlug(): string {
    // genuinely local async state — AsyncStorage has no reactive primitive
    const [slug, setSlug] = useState(cachedNativeSlug ?? '')

    useEffect(() => {
        if (Platform.OS === 'web' || cachedNativeSlug != null) return
        readNativeSlug().then(s => {
            if (s) setSlug(s)
        })
    }, [])

    if (Platform.OS === 'web') return WEB_ORG_SLUG
    return slug
}
