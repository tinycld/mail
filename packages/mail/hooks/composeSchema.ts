import { z } from '~/ui/form'

const emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

export const composeSchema = z.object({
    to: z
        .string()
        .min(1, 'At least one recipient is required')
        .refine(
            value =>
                value
                    .split(',')
                    .map(s => s.trim())
                    .filter(Boolean)
                    .every(email => emailPattern.test(email)),
            'One or more email addresses are invalid'
        ),
    subject: z.string().min(1, 'Subject is required'),
})

export type ComposeFormData = z.infer<typeof composeSchema>

export function parseRecipients(value: string) {
    return value
        .split(',')
        .map(s => s.trim())
        .filter(Boolean)
        .map(email => ({ name: '', email }))
}
