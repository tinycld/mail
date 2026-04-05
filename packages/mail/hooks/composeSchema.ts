import { z } from '~/ui/form'

const emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
const namedEmailPattern = /^(.+)\s*<([^\s@]+@[^\s@]+\.[^\s@]+)>$/

function isValidRecipient(segment: string) {
    const namedMatch = segment.match(namedEmailPattern)
    if (namedMatch) return emailPattern.test(namedMatch[2])
    return emailPattern.test(segment)
}

function validateRecipients(value: string) {
    if (!value) return true
    return value
        .split(',')
        .map(s => s.trim())
        .filter(Boolean)
        .every(isValidRecipient)
}

export const composeSchema = z.object({
    to: z
        .string()
        .min(1, 'At least one recipient is required')
        .refine(validateRecipients, 'One or more email addresses are invalid'),
    cc: z.string().refine(validateRecipients, 'One or more Cc addresses are invalid'),
    bcc: z.string().refine(validateRecipients, 'One or more Bcc addresses are invalid'),
    subject: z.string().min(1, 'Subject is required'),
})

export type ComposeFormData = z.infer<typeof composeSchema>

export function parseRecipients(value: string) {
    return value
        .split(',')
        .map(s => s.trim())
        .filter(Boolean)
        .map(segment => {
            const namedMatch = segment.match(namedEmailPattern)
            if (namedMatch) return { name: namedMatch[1].trim(), email: namedMatch[2] }
            return { name: '', email: segment }
        })
}
