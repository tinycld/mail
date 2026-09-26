import type { SendEmailRequest } from '@tinycld/app-generated/mail-api'

export function hasVerifiedDomain(rows: ReadonlyArray<{ verified: boolean }>): boolean {
    return rows.some(row => row.verified)
}

// Only a verified domain can take a mailbox that sends and receives, so the
// first address is offered on those alone.
export function verifiedDomainOptions(
    rows: ReadonlyArray<{ id: string; domain: string; verified: boolean }>
): Array<{ label: string; value: string }> {
    return rows.filter(row => row.verified).map(row => ({ label: row.domain, value: row.id }))
}

export function isMailboxMember(
    memberships: ReadonlyArray<{ user: string }>,
    userId: string | undefined
): boolean {
    if (!userId) return false
    return memberships.some(m => m.user === userId)
}

export function testMessageRequest({
    mailboxId,
    to,
    workspaceName,
}: {
    mailboxId: string
    to: string
    workspaceName: string
}): SendEmailRequest {
    const name = workspaceName.trim() || 'your workspace'
    const text = `This is a test message from ${name}. Your new address can send email.`
    return {
        mailbox_id: mailboxId,
        to: [{ email: to, name: '' }],
        subject: `Test message from ${name}`,
        text_body: text,
        html_body: `<p>${escapeHtml(text)}</p>`,
    }
}

function escapeHtml(value: string): string {
    return value
        .replaceAll('&', '&amp;')
        .replaceAll('<', '&lt;')
        .replaceAll('>', '&gt;')
        .replaceAll('"', '&quot;')
}
