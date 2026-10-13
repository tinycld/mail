import { z } from '@tinycld/core/ui/form'

interface TakenAddresses {
    mailboxAddresses: ReadonlySet<string>
    aliasAddresses: ReadonlySet<string>
}

// Collisions are checked against the domain's rows already in the store; the
// server's unique index still rejects a race between two admins.
export function aliasSchemaFor({ mailboxAddresses, aliasAddresses }: TakenAddresses) {
    return z.object({
        address: z
            .string()
            .trim()
            .toLowerCase()
            .min(1, 'Address is required')
            .max(64)
            .regex(/^[a-z0-9._-]+$/, 'Only lowercase letters, numbers, dots, hyphens, underscores')
            .refine(address => !mailboxAddresses.has(address), {
                message: 'Address is already a primary mailbox on this domain',
            })
            .refine(address => !aliasAddresses.has(address), {
                message: 'Address is already an alias on this domain',
            }),
    })
}

export type AliasFormValues = z.infer<ReturnType<typeof aliasSchemaFor>>
