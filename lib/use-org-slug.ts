import AsyncStorage from '@react-native-async-storage/async-storage'
import { useEffect, useState } from 'react'
import { Platform } from 'react-native'

const PRIMARY_ORG_STORAGE_KEY = 'tinycld_primary_org'

function getOrgSlugFromHostname(): string {
    if (typeof window === 'undefined') return ''
    const hostname = window.location.hostname

    // acme.localhost → acme
    if (hostname.endsWith('.localhost')) {
        return hostname.slice(0, -'.localhost'.length)
    }

    // acme.tinycld.com → acme
    const parts = hostname.split('.')
    if (parts.length >= 3) {
        return parts[0]
    }

    // bare domain (localhost, tinycld.com) → no org
    return ''
}

export function useOrgSlug(): string {
    const [nativeSlug, setNativeSlug] = useState('')

    useEffect(() => {
        if (Platform.OS !== 'web') {
            AsyncStorage.getItem(PRIMARY_ORG_STORAGE_KEY).then(slug => {
                if (slug) setNativeSlug(slug)
            })
        }
    }, [])

    if (Platform.OS === 'web') {
        return getOrgSlugFromHostname()
    }

    return nativeSlug
}
