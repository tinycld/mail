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

// Which domain the step's DNS + Verify panel is for. The domain added in this
// visit wins, verified or not, so the person sees it turn green. Otherwise it
// is the newest unverified domain, so a person who comes back to the wizard
// can still finish one they added earlier. `rows` are oldest first.
export function domainPanelTarget<T extends { id: string; verified: boolean }>(
    rows: readonly T[],
    addedId: string | undefined
): T | undefined {
    if (addedId) return rows.find(row => row.id === addedId)
    return rows.filter(row => !row.verified).at(-1)
}

// The add endpoint answers 503 when this deployment has no mail provider to
// enroll the domain with. The Email sending step sets one up — unless the mail
// settings are administered elsewhere, and then only that administrator can.
export function setupAddDomainError(error: unknown, isMailManaged: boolean): string | null {
    const status =
        typeof error === 'object' && error !== null && 'status' in error ? error.status : undefined
    if (status !== 503) return null
    if (isMailManaged) return 'Mail domains are not available yet. Ask your administrator.'
    return 'Set up email sending first, then come back to this step.'
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
