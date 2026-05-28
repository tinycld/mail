import { eq } from '@tanstack/db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { captureMessageToSentry } from '@tinycld/core/lib/sentry'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useOrgInfo } from '@tinycld/core/lib/use-org-info'
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

    const { orgId, orgSlug } = useOrgInfo()
    const { userOrgId } = useCurrentRole()

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

    const lastSnapshotRef = useRef<string>('')
    useEffect(() => {
        const snapshot = JSON.stringify({
            orgId,
            orgSlug,
            userOrgId,
            membersLen: members?.length ?? null,
            mailboxId,
            domainId,
            domainVerified: domain?.verified ?? null,
        })
        if (snapshot === lastSnapshotRef.current) return
        lastSnapshotRef.current = snapshot
        captureMessageToSentry('mail-send-readiness', 'state-change', {
            orgId,
            orgSlug,
            userOrgId,
            membersLen: members?.length ?? null,
            mailboxId,
            domainId,
            domainVerified: domain?.verified ?? null,
        })
    }, [orgId, orgSlug, userOrgId, members, mailboxId, domainId, domain])

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
