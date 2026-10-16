import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'

// Rank is the position in the dragged order, starting at 1; the server picks
// the main domain from it.
export function useReorderDomains() {
    const [domainsCollection] = useStore('mail_domains')
    return useMutation({
        mutationFn: mutation(function* (orderedIds: string[]) {
            yield orderedIds.map((id, index) =>
                domainsCollection.update(id, d => {
                    d.rank = index + 1
                })
            )
        }),
    })
}
