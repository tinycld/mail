import { z } from '@tinycld/core/ui/form'

export const providerSchema = z
    .object({
        provider: z.enum(['postmark', 'smtp']),
        // Secrets — blank means "leave the stored value unchanged" (write-only).
        postmark_server_token: z.string(),
        postmark_account_token: z.string(),
        smtp_public_hostname: z.string().trim(),
        smtp_dkim_selector: z.string().trim(),
        smtp_inbound_mode: z.enum(['', 'smtp', 'imap']),
        smtp_imap_host: z.string().trim(),
        smtp_imap_port: z.number().int().min(0).max(65535),
        smtp_imap_username: z.string().trim(),
        smtp_imap_password: z.string(), // secret
        smtp_imap_mailbox: z.string().trim(),
        smtp_imap_poll_interval_seconds: z.number().int().min(0),
        smtp_imap_use_tls: z.boolean(),
    })
    .superRefine((data, ctx) => {
        if (data.provider !== 'smtp') return
        // The built-in listener is the MX: domain verification expects each
        // domain's MX to point here, and a blank hostname falls back to
        // "localhost", which no domain can publish.
        if (data.smtp_inbound_mode === 'smtp' && !data.smtp_public_hostname) {
            ctx.addIssue({
                code: 'custom',
                message: 'Public hostname is required when this server receives mail over SMTP',
                path: ['smtp_public_hostname'],
            })
        }
        // The IMAP fetcher does not start without a host.
        if (data.smtp_inbound_mode === 'imap' && !data.smtp_imap_host) {
            ctx.addIssue({
                code: 'custom',
                message: 'IMAP host is required to poll a mailbox',
                path: ['smtp_imap_host'],
            })
        }
    })

export type ProviderFormValues = z.infer<typeof providerSchema>
