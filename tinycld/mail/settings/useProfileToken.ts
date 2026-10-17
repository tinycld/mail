import { useQuery } from '@tanstack/react-query'
import { pb } from '@tinycld/core/lib/pocketbase'

const REFRESH_MS = 5 * 60_000

// The token lives 10 minutes. Refreshing every 5 keeps one in hand, so the
// button opens the link in the same press: a window opened after an await is
// blocked as a pop-up on web.
export function useProfileToken() {
    return useQuery({
        queryKey: ['mail', 'profile-token'],
        queryFn: async () => {
            const { token } = await pb.send<{ token: string }>('/api/mail/mobileconfig/token', {
                method: 'POST',
            })
            return token
        },
        staleTime: REFRESH_MS,
        refetchInterval: REFRESH_MS,
        enabled: pb.authStore.isValid,
    })
}
