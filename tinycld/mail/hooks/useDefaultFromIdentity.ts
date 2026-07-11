import { useMemo } from 'react'
import { type FromIdentity, filterOwnAddresses, pickDefaultFrom } from './defaultFromIdentity'
import { useSendableIdentities } from './useSendableIdentities'

export type { FromIdentity } from './defaultFromIdentity'
export { filterOwnAddresses, pickDefaultFrom }

interface UseDefaultFromIdentityParams {
    mode: 'new' | 'reply' | 'forward'
    replyToAddresses?: string[]
}

export function useDefaultFromIdentity({
    mode,
    replyToAddresses,
}: UseDefaultFromIdentityParams): FromIdentity {
    const identities = useSendableIdentities()
    // Callers typically pass a fresh `replyToAddresses` array each render, so we
    // key the memo on a stable primitive derived from its contents. The raw
    // strings are recovered inside the memo by splitting that key; casing is
    // irrelevant because `pickDefaultFrom` lowercases via `extractBareAddress`.
    const key = (replyToAddresses ?? []).join('\x00').toLowerCase()
    return useMemo(() => {
        const addresses = key ? key.split('\x00') : []
        return pickDefaultFrom({ mode, identities, replyToAddresses: addresses })
    }, [mode, identities, key])
}
