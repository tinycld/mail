import type { SendEmailRequest } from '@tinycld/app-generated/mail-api'
import { appHref } from '@tinycld/core/lib/org-routes'
import { type DomainChecks, isDomainReady } from '../settings/domain-ready'
import { testMessageHtml, testMessageSubject, testMessageText } from './test-message'

export function hasReadyDomain(rows: readonly DomainChecks[]): boolean {
    return rows.some(isDomainReady)
}

// Only a ready domain can take a mailbox that sends and receives, so the
// first address is offered on those alone.
export function readyDomainOptions(
    rows: ReadonlyArray<DomainChecks & { id: string; domain: string }>
): Array<{ label: string; value: string }> {
    return rows.filter(isDomainReady).map(row => ({ label: row.domain, value: row.id }))
}

// Which domain the step's DNS + Verify panel is for. The domain added in this
// visit wins, ready or not, so the person sees it turn green. Otherwise it
// is the newest domain that is not ready, so a person who comes back to the
// wizard can still finish one they added earlier. `rows` are oldest first.
export function domainPanelTarget<T extends DomainChecks & { id: string }>(
    rows: readonly T[],
    addedId: string | undefined
): T | undefined {
    if (addedId) return rows.find(row => row.id === addedId)
    return rows.filter(row => !isDomainReady(row)).at(-1)
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
    fromAddress,
    to,
    workspaceName,
}: {
    mailboxId: string
    fromAddress: string
    to: string
    workspaceName: string
}): SendEmailRequest {
    const name = workspaceName.trim() || 'your workspace'
    return {
        mailbox_id: mailboxId,
        to: [{ email: to, name: '' }],
        subject: testMessageSubject(name),
        text_body: testMessageText(name, fromAddress),
        html_body: testMessageHtml(name, fromAddress),
    }
}

export const EMAIL_DOMAIN_STEP_HREF = appHref('setup/mail.email-domain')

export type TestMessageState =
    | { kind: 'none' }
    | { kind: 'sending' }
    | { kind: 'sent' }
    | { kind: 'delivered' }
    | { kind: 'bounced'; bounceClass: string; reason: string }

// An outbound message never gets delivery_status "delivered": the provider's
// delivery notice sets delivered_at instead, so that field decides delivery.
export function testMessageState(
    row:
        | {
              delivery_status: string
              delivered_at: string
              bounce_class: string
              bounce_reason: string
          }
        | undefined
): TestMessageState {
    if (!row) return { kind: 'none' }
    if (row.delivery_status === 'bounced' || row.delivery_status === 'spam_complaint') {
        return { kind: 'bounced', bounceClass: row.bounce_class, reason: row.bounce_reason }
    }
    if (row.delivered_at) return { kind: 'delivered' }
    if (row.delivery_status === 'sending') return { kind: 'sending' }
    return { kind: 'sent' }
}

export function testMessageLabel(state: TestMessageState): string {
    switch (state.kind) {
        case 'none':
            return ''
        case 'sending':
            return 'Sending…'
        case 'sent':
            return 'Sent. Waiting for delivery…'
        case 'delivered':
            return 'Delivered'
        case 'bounced': {
            const prefix = state.bounceClass ? `Bounced (${state.bounceClass})` : 'Bounced'
            return state.reason ? `${prefix}: ${state.reason}` : prefix
        }
    }
}
