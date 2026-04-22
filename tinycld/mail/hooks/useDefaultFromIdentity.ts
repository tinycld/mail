import { useMemo } from 'react'
import { type FromIdentity, filterOwnAddresses, pickDefaultFrom } from './defaultFromIdentity'
import { useSendableIdentities } from './useSendableIdentities'

export { filterOwnAddresses, pickDefaultFrom }
export type { FromIdentity } from './defaultFromIdentity'

interface UseDefaultFromIdentityParams {
    mode: 'new' | 'reply' | 'forward'
    replyToAddresses?: string[]
}

export function useDefaultFromIdentity({
    mode,
    replyToAddresses = [],
}: UseDefaultFromIdentityParams): FromIdentity {
    const identities = useSendableIdentities()
    return useMemo(
        () => pickDefaultFrom({ mode, identities, replyToAddresses }),
        [mode, identities, replyToAddresses]
    )
}
