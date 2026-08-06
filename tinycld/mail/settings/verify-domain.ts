import type { VerifyDomainResponse } from '@tinycld/app-generated/mail-api'

// The verify endpoint returns 200 even when persisting the check results
// failed — the checks themselves ran. `saved: false` means the record still
// holds the previous state, so treating the mutation as successful would show
// stale verification as fresh. Throw so the failure surfaces through the
// mutation's error state.
export function assertVerifySaved(res: VerifyDomainResponse): VerifyDomainResponse {
    if (!res.saved) {
        throw new Error(res.save_error || 'Verification ran but its result could not be saved')
    }
    return res
}
