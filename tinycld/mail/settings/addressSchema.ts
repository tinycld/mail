import { z } from '@tinycld/core/ui/form'

interface AddressRules {
    // `${localPart}@${domainId}` of every existing address that could collide.
    taken: ReadonlySet<string>
    // Email names of people other than this mailbox's owner.
    reservedNames: ReadonlySet<string>
    // Domains with a rule (everyone or groups), where email names are reserved.
    ruleDomainIds: ReadonlySet<string>
}

// The server checks the same rules (server/addresses.go checkAddressAvailable);
// these catch the common case before the round trip.
export function addressSchemaFor({ taken, reservedNames, ruleDomainIds }: AddressRules) {
    return z
        .object({
            local_part: z
                .string()
                .trim()
                .toLowerCase()
                .min(1, 'Address is required')
                .max(64)
                .regex(
                    /^[a-z0-9._-]+$/,
                    'Only lowercase letters, numbers, dots, hyphens, underscores'
                ),
            domain: z.string().min(1, 'Choose a domain'),
        })
        .superRefine((value, ctx) => {
            if (taken.has(`${value.local_part}@${value.domain}`)) {
                ctx.addIssue({
                    code: 'custom',
                    path: ['local_part'],
                    message: 'This address already exists',
                })
                return
            }
            if (ruleDomainIds.has(value.domain) && reservedNames.has(value.local_part)) {
                ctx.addIssue({
                    code: 'custom',
                    path: ['local_part'],
                    message:
                        "This is a person's email name, and this domain's rule keeps that name for them",
                })
            }
        })
}

export type AddressFormValues = z.infer<ReturnType<typeof addressSchemaFor>>
