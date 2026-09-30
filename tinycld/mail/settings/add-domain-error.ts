import { errorToString, extractValidationErrors } from '@tinycld/core/lib/errors'

// The add-domain form has one field, so every refusal from the endpoint — a
// field validation error or a plain one like a 409 for a domain already on
// this host — belongs under that field, not in a toast.
export function addDomainErrorMessage(
    error: unknown,
    describeError?: (error: unknown) => string | null
): string {
    return describeError?.(error) ?? extractValidationErrors(error)?.domain ?? errorToString(error)
}
