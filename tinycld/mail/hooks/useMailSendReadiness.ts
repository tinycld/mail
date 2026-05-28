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

    // Identity-based resolution: `mailboxes` from a prior render's filter may
    // still be in the result set while the query catches up to the new
    // `mailboxId`. Picking by id (rather than [0]) means a stale row from a
    // previous filter doesn't masquerade as the current mailbox. While the
    // requested mailbox isn't in the result yet, `mailbox` is null AND the
    // mailboxesLoading flag below treats it as loading (no false "no-domain").
    const mailbox = mailboxes?.find(m => m.id === mailboxId) ?? null
    const domainId = mailbox?.domain ?? null

    const { data: domains } = useOrgLiveQuery(
        query =>
            query
                .from({ mail_domains: domainsCollection })
                .where(({ mail_domains }) => eq(mail_domains.id, domainId ?? '')),
        [domainId]
    )

    const domain = domains?.find(d => d.id === domainId) ?? null

    // Suppress blockers while any dependent query is still loading OR catching
    // up to its current filter. "Loading" now means: the query hasn't returned
    // yet (data === undefined), OR the requested row isn't in the result yet
    // (data returned but doesn't contain the id we asked for — i.e. a stale
    // prior filter's result). Without the second condition, an FK that hasn't
    // resolved looks like a legitimate "no FK set" and trips a false blocker.
    const membersLoading = members === undefined
    const mailboxesLoading = mailboxId !== null && (mailboxes === undefined || mailbox === null)
    const domainsLoading = domainId !== null && (domains === undefined || domain === null)
    const isSuppressed = membersLoading || mailboxesLoading || domainsLoading

    const lastSuppressionRef = useRef<string>('')
    useEffect(() => {
        if (!isSuppressed) {
            lastSuppressionRef.current = ''
            return
        }
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
            mailboxRowMatched: mailbox !== null,
            domainsResolved: domains !== undefined,
            domainRowMatched: domain !== null,
        })
    }, [
        isSuppressed,
        membersLoading,
        mailboxesLoading,
        domainsLoading,
        mailboxId,
        domainId,
        members,
        mailboxes,
        domains,
        mailbox,
        domain,
    ])

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
