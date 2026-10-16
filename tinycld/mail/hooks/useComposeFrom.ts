import { useComposeStore } from '../stores/compose-store'
import { resolveComposeState } from './composeFrom'
import { useSendableIdentities } from './useSendableIdentities'

/**
 * The From choices a compose surface offers and the From it sends; see
 * resolveComposeState. `lockedMailboxId` is the open draft's mailbox.
 */
export function useComposeFrom(fallbackMailboxId: string, lockedMailboxId: string | null = null) {
    const identities = useSendableIdentities()
    const storeMailboxId = useComposeStore(s => s.mailboxId)
    const storeAddressId = useComposeStore(s => s.fromAddressId)
    const setFrom = useComposeStore(s => s.setFrom)
    const { choices, from } = resolveComposeState({
        identities,
        storeMailboxId,
        storeAddressId,
        fallbackMailboxId,
        lockedMailboxId,
    })
    return { choices, from, setFrom }
}
