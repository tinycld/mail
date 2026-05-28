import { eq } from '@tanstack/db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { captureMessageToSentry } from '@tinycld/core/lib/sentry'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import { useEffect, useRef } from 'react'

export type MailSendBlocker = 'no-mailbox' | 'no-domain' | 'domain-unverified'

export interface MailSendReadiness {
    mailboxId: string | null
    blocker: MailSendBlocker | null
    message: string | null
}

export function useMailSendReadiness(): MailSendReadiness {
    const [membersCollection, mailboxesCollection, domainsCollection] = useStore(
        'mail_mailbox_members',
        'mail_mailboxes',
        'mail_domains'
    )

    const { data: members } = useOrgLiveQuery((query, { userOrgId }) =>
        query
            .from({ mail_mailbox_members: membersCollection })
            .where(({ mail_mailbox_members }) => eq(mail_mailbox_members.user_org, userOrgId))
    )

    const mailboxId = members?.[0]?.mailbox ?? null

    const { data: mailboxes } = useOrgLiveQuery(
        query =>
            query
                .from({ mail_mailboxes: mailboxesCollection })
                .where(({ mail_mailboxes }) => eq(mail_mailboxes.id, mailboxId ?? '')),
        [mailboxId]
    )

    const domainId = mailboxes?.[0]?.domain ?? null

    const { data: domains } = useOrgLiveQuery(
        query =>
            query
                .from({ mail_domains: domainsCollection })
                .where(({ mail_domains }) => eq(mail_domains.id, domainId ?? '')),
        [domainId]
    )

    const domain = domains?.[0] ?? null

    // Suppress blockers while any dependent query is still loading. Without
    // this, the user gets toasted on transient load order (e.g. members
    // resolved but mailboxes not yet) for problems that don't actually exist.
    const membersLoading = members === undefined
    const mailboxesLoading = mailboxId !== null && mailboxes === undefined
    const domainsLoading = domainId !== null && domains === undefined
    const isSuppressed = membersLoading || mailboxesLoading || domainsLoading

    const lastSuppressionRef = useRef<string>('')
    useEffect(() => {
        if (!isSuppressed) {
            lastSuppressionRef.current = ''
            return
        }
        // Log each distinct suppression state once so we can see in prod how
        // often the toast would have fired on a transient load state.
        const which: ('members' | 'mailboxes' | 'domains')[] = []
        if (membersLoading) which.push('members')
        if (mailboxesLoading) which.push('mailboxes')
        if (domainsLoading) which.push('domains')
        const key = which.join(',')
        if (key === lastSuppressionRef.current) return
        lastSuppressionRef.current = key
        captureMessageToSentry('mail-send-readiness', 'blocker-suppressed-loading', {
            loading: which,
            mailboxId,
            domainId,
            membersResolved: members !== undefined,
            mailboxesResolved: mailboxes !== undefined,
            domainsResolved: domains !== undefined,
        })
    }, [isSuppressed, membersLoading, mailboxesLoading, domainsLoading, mailboxId, domainId, members, mailboxes, domains])

    if (isSuppressed) {
        return { mailboxId, blocker: null, message: null }
    }

    if (!mailboxId) {
        return {
            mailboxId: null,
            blocker: 'no-mailbox',
            message: 'No mailbox found. Ask your admin to add you to a mailbox.',
        }
    }

    if (!domain) {
        return {
            mailboxId,
            blocker: 'no-domain',
            message:
                'Mailbox exists but its sending domain is missing. An admin must configure a mail domain.',
        }
    }

    if (!domain.verified) {
        return {
            mailboxId,
            blocker: 'domain-unverified',
            message: `The domain "${domain.domain}" is not verified. Outgoing mail will fail until an admin completes verification in Settings → Mail.`,
        }
    }

    return { mailboxId, blocker: null, message: null }
}
