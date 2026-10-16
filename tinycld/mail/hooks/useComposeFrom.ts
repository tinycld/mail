import { useComposeStore } from '../stores/compose-store'
import { resolveComposeFrom } from './composeFrom'
import { useSendableIdentities } from './useSendableIdentities'

/** The From a compose surface shows and sends; see resolveComposeFrom. */
export function useComposeFrom(fallbackMailboxId: string) {
    const identities = useSendableIdentities()
    const mailboxId = useComposeStore(s => s.mailboxId)
    const fromAddressId = useComposeStore(s => s.fromAddressId)
    const setFrom = useComposeStore(s => s.setFrom)
    const from = resolveComposeFrom(identities, mailboxId, fromAddressId, fallbackMailboxId)
    return { identities, from, setFrom }
}
