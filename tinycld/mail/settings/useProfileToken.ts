import { useQuery } from '@tanstack/react-query'
import { log } from '@tinycld/core/lib/logger'
import { pb } from '@tinycld/core/lib/pocketbase'

const REFRESH_MS = 5 * 60_000
// A token lives 10 minutes. A cached one older than this may be about to
// expire, or already expired after a long time in the background on native.
const MAX_AGE_MS = 9 * 60_000

// The token lives 10 minutes. Refreshing every 5 keeps one in hand, so the
// button opens the link in the same press: a window opened after an await is
// blocked as a pop-up on web.
export function useProfileToken() {
    const { data, dataUpdatedAt, isError, refetch } = useQuery({
        queryKey: ['mail', 'profile-token'],
        queryFn: async () => {
            try {
                const { token } = await pb.send<{ token: string }>('/api/mail/mobileconfig/token', {
                    method: 'POST',
                })
                return token
            } catch (err) {
                log.warn('mail.client-setup', 'profile token fetch failed', {
                    error: String(err),
                })
                throw err
            }
        },
        staleTime: REFRESH_MS,
        refetchInterval: REFRESH_MS,
        // The next refresh is the retry; a retry here would log the same failure again.
        retry: false,
        enabled: pb.authStore.isValid,
    })

    // The refetch failure is logged by the query function.
    const withFreshToken = (use: (token: string) => void) => {
        if (data && Date.now() - dataUpdatedAt < MAX_AGE_MS) {
            use(data)
            return
        }
        refetch().then(result => {
            // A failed refetch keeps the old token in data, which is the one to avoid.
            if (result.isSuccess) use(result.data)
        })
    }

    return { hasToken: Boolean(data), isError, withFreshToken }
}
