import { eq } from '@tanstack/db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import { useMemo } from 'react'
import { groupSendableIdentities } from './flattenSendableIdentities'

export type { SendableIdentity } from './flattenSendableIdentities'

export function useSendableIdentities() {
    const [membersCollection, mailboxesCollection, domainsCollection, aliasesCollection] = useStore(
        'mail_mailbox_members',
        'mail_mailboxes',
        'mail_domains',
        'mail_mailbox_aliases'
    )

    // One query: membership → mailbox → domain resolve in the same expression
    // (a mailbox whose domain is gone drops out, as before), with aliases
    // left-joined so a mailbox without any still yields an identity.
    const { data: rows } = useOrgLiveQuery((query, { userId }) =>
        query
            .from({ member: membersCollection })
            .innerJoin({ mailbox: mailboxesCollection }, ({ member, mailbox }) =>
                eq(member.mailbox, mailbox.id)
            )
            .innerJoin({ domain: domainsCollection }, ({ mailbox, domain }) =>
                eq(mailbox.domain, domain.id)
            )
            .leftJoin({ alias: aliasesCollection }, ({ mailbox, alias }) =>
                eq(alias.mailbox, mailbox.id)
            )
            .where(({ member }) => eq(member.user, userId))
    )

    return useMemo(() => groupSendableIdentities(rows ?? []), [rows])
}
