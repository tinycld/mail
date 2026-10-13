import type { z } from '@tinycld/core/ui/form'
import { describe, expect, it } from 'vitest'
import { providerSchema } from '~/tinycld/mail/system-settings/providerSchema'

const base: z.input<typeof providerSchema> = {
    provider: 'smtp',
    postmark_server_token: '',
    postmark_account_token: '',
    smtp_public_hostname: '',
    smtp_dkim_selector: '',
    smtp_inbound_mode: '',
    smtp_imap_host: '',
    smtp_imap_port: 0,
    smtp_imap_username: '',
    smtp_imap_password: '',
    smtp_imap_mailbox: 'INBOX',
    smtp_imap_poll_interval_seconds: 0,
    smtp_imap_use_tls: true,
}

const issuePaths = (values: z.input<typeof providerSchema>) =>
    providerSchema.safeParse(values).error?.issues.map(i => i.path.join('.')) ?? []

describe('providerSchema', () => {
    it('accepts outbound-only SMTP without a hostname or IMAP host', () => {
        expect(issuePaths(base)).toEqual([])
    })

    it('requires a public hostname when the built-in listener receives mail', () => {
        expect(issuePaths({ ...base, smtp_inbound_mode: 'smtp' })).toEqual(['smtp_public_hostname'])
        expect(
            issuePaths({ ...base, smtp_inbound_mode: 'smtp', smtp_public_hostname: '   ' })
        ).toEqual(['smtp_public_hostname'])
        expect(
            issuePaths({ ...base, smtp_inbound_mode: 'smtp', smtp_public_hostname: 'mx.acme.com' })
        ).toEqual([])
    })

    it('requires an IMAP host when polling a mailbox', () => {
        expect(issuePaths({ ...base, smtp_inbound_mode: 'imap' })).toEqual(['smtp_imap_host'])
        expect(
            issuePaths({ ...base, smtp_inbound_mode: 'imap', smtp_imap_host: 'imap.acme.com' })
        ).toEqual([])
    })

    it('ignores SMTP fields when the provider is Postmark', () => {
        expect(issuePaths({ ...base, provider: 'postmark', smtp_inbound_mode: 'imap' })).toEqual([])
    })
})
