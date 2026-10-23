import { z } from '@tinycld/core/ui/form'

export interface AddressRules {
    // `${localPart}@${domainId}` of every existing address that could collide.
    taken: ReadonlySet<string>
    // Email names of people other than this mailbox's owner.
    reservedNames: ReadonlySet<string>
    // Domains with a rule (everyone or groups), where email names are reserved.
    ruleDomainIds: ReadonlySet<string>
}

export type AddressIssue = 'taken' | 'reserved'

// The server checks the same rules (server/addresses.go checkAddressAvailable);
// these catch the common case before the round trip.
export function addressIssue(
    { taken, reservedNames, ruleDomainIds }: AddressRules,
    localPart: string,
    domainId: string
): AddressIssue | null {
    if (taken.has(`${localPart}@${domainId}`)) return 'taken'
    if (ruleDomainIds.has(domainId) && reservedNames.has(localPart)) return 'reserved'
    return null
}

export const localPartSchema = z
    .string()
    .trim()
    .toLowerCase()
    .min(1, 'Address is required')
    .max(64)
    .regex(/^[a-z0-9._-]+$/, 'Only lowercase letters, numbers, dots, hyphens, underscores')

export function addressSchemaFor(rules: AddressRules) {
    return z
        .object({
            local_part: localPartSchema,
            domain: z.string().min(1, 'Choose a domain'),
        })
        .superRefine((value, ctx) => {
            const issue = addressIssue(rules, value.local_part, value.domain)
            if (issue === 'taken') {
                ctx.addIssue({
                    code: 'custom',
                    path: ['local_part'],
                    message: 'This address already exists',
                })
            }
            if (issue === 'reserved') {
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
