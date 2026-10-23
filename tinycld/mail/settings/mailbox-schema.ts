import { z } from '@tinycld/core/ui/form'
import { type AddressRules, addressIssue, localPartSchema } from './addressSchema'

const displayNameSchema = z.string().min(1, 'Display name is required').max(200)

export const mailboxSchema = z.object({
    address: localPartSchema,
    domain: z.string().min(1, 'Domain is required'),
    display_name: displayNameSchema,
})

export type MailboxFormValues = z.infer<typeof mailboxSchema>

// A shared inbox may take its address on several domains at once. Each one
// must be free, so the rules are checked per chosen domain and the message
// names the full address that collides.
export function sharedMailboxSchemaFor(
    rules: AddressRules,
    domainNames: ReadonlyMap<string, string>
) {
    return z
        .object({
            address: localPartSchema,
            domains: z.array(z.string()).min(1, 'Choose at least one domain'),
            display_name: displayNameSchema,
        })
        .superRefine((value, ctx) => {
            for (const domain of value.domains) {
                const issue = addressIssue(rules, value.address, domain)
                if (!issue) continue
                const full = `${value.address}@${domainNames.get(domain) ?? domain}`
                ctx.addIssue({
                    code: 'custom',
                    path: ['address'],
                    message:
                        issue === 'taken'
                            ? `${full} already exists`
                            : `${full} is a person's email name, and that domain's rule keeps it for them`,
                })
                return
            }
        })
}

export type SharedMailboxFormValues = z.infer<ReturnType<typeof sharedMailboxSchemaFor>>
