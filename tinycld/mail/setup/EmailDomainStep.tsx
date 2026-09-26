import { useLiveQuery } from '@tanstack/react-db'
import type { AddDomainResponse, OutboundCheckResult } from '@tinycld/app-generated/mail-api'
import { SidebarSlot } from '@tinycld/core/components/sidebar-primitives/SidebarSlot'
import { errorToString } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { pb, useStore } from '@tinycld/core/lib/pocketbase'
import type { SetupStepProps } from '@tinycld/core/lib/setup/types'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { useState } from 'react'
import { Text, View } from 'react-native'
import { DnsRecordsPanel } from '~/tinycld/mail/settings/DnsRecordsPanel'
import { AddDomainForm } from '~/tinycld/mail/settings/provider'
import { assertVerifySaved } from '~/tinycld/mail/settings/verify-domain'
import { hasVerifiedDomain } from '~/tinycld/mail/setup/setup-logic'

// Adding and verifying domains is an owner/admin action on the server.
export function useIsStepVisible() {
    const { isReady, isAdmin } = useCurrentRole()
    return isReady ? isAdmin : undefined
}

export function useIsStepDone() {
    const [domainsCollection] = useStore('mail_domains')
    const { data, isReady } = useLiveQuery(query =>
        query.from({ d: domainsCollection }).select(({ d }) => ({ verified: d.verified }))
    )
    return isReady ? hasVerifiedDomain(data ?? []) : undefined
}

function useDomains() {
    const [domainsCollection] = useStore('mail_domains')
    const { data = [] } = useLiveQuery(query =>
        query
            .from({ d: domainsCollection })
            .orderBy(({ d }) => d.created, 'asc')
            .select(({ d }) => ({
                id: d.id,
                domain: d.domain,
                verified: d.verified,
                details: d.verification_details,
            }))
    )
    return data.map(({ details, ...d }) => ({ ...d, outbound: details?.outbound }))
}

type DomainItem = ReturnType<typeof useDomains>[number]

function useVerifyDomain() {
    const verify = useMutation({
        mutationFn: async (id: string) =>
            assertVerifySaved(await pb.send(`/api/mail/domains/${id}/verify`, { method: 'POST' })),
    })
    return {
        verify: verify.mutate,
        isPending: verify.isPending,
        errorMessage: verify.error ? errorToString(verify.error) : null,
    }
}

// The domain added in this step. Its DNS records come from the live row once
// a verify has refreshed them, and from the add response until then.
function useNewDomain(domains: DomainItem[]) {
    const [added, setAdded] = useState<AddDomainResponse | null>(null)
    const row = domains.find(d => d.id === added?.id)
    const outbound: OutboundCheckResult | undefined = row?.outbound ?? added?.records
    return { added, setAdded, outbound, isVerified: row?.verified ?? false }
}

function VerifiedLabel({ isVerified }: { isVerified: boolean }) {
    if (isVerified) return <Text className="text-xs text-success">Verified</Text>
    return <Text className="text-xs text-muted-foreground">Not verified</Text>
}

function ErrorText({ message }: { message: string | null }) {
    if (!message) return null
    return <Text className="text-sm text-danger">{message}</Text>
}

function NewDomainPanel({
    added,
    outbound,
    isVerified,
}: {
    added: AddDomainResponse | null
    outbound: OutboundCheckResult | undefined
    isVerified: boolean
}) {
    const { verify, isPending, errorMessage } = useVerifyDomain()
    if (!added) return null
    return (
        <View testID="setup-new-domain" className="mb-4 gap-2 rounded-xl border border-border p-3">
            <Text className="text-sm text-foreground">
                Publish these records at your DNS provider for {added.domain}, then verify. DNS
                changes can take some time to show.
            </Text>
            <DnsRecordsPanel outbound={outbound} isVisible />
            <View className="flex-row items-center gap-3">
                <Button
                    variant="outline"
                    className="self-start"
                    onPress={() => verify(added.id)}
                    isDisabled={isPending}
                >
                    <ButtonText>{isPending ? 'Verifying…' : 'Verify'}</ButtonText>
                </Button>
                <VerifiedLabel isVerified={isVerified} />
            </View>
            <ErrorText message={errorMessage} />
        </View>
    )
}

function DomainRow({ domain }: { domain: DomainItem }) {
    return (
        <View className="flex-row items-center justify-between border-b border-border py-2">
            <Text className="text-sm text-foreground">{domain.domain}</Text>
            <VerifiedLabel isVerified={domain.verified} />
        </View>
    )
}

function DomainList({ domains }: { domains: DomainItem[] }) {
    if (domains.length === 0) return null
    const rows = domains.map(d => <DomainRow key={d.id} domain={d} />)
    return (
        <View className="mb-4">
            <Text className="text-sm font-semibold text-foreground">Your domains</Text>
            {rows}
        </View>
    )
}

export default function EmailDomainStep({ next }: SetupStepProps) {
    const domains = useDomains()
    const { added, setAdded, outbound, isVerified } = useNewDomain(domains)
    return (
        <View className="max-w-[440px] gap-1">
            <Text className="text-2xl font-bold text-foreground">Your email domain</Text>
            <Text className="mb-3 text-sm text-muted-foreground">
                Choose where your team's email addresses live. You can add more domains later in
                Settings → Mail → Domains.
            </Text>
            <SidebarSlot target="mail" slot="setup-domain-options" />
            <Text className="mt-2 text-sm font-semibold text-foreground">Use my own domain</Text>
            <AddDomainForm onAdded={setAdded} />
            <NewDomainPanel added={added} outbound={outbound} isVerified={isVerified} />
            <DomainList domains={domains} />
            <Button className="self-start" onPress={next}>
                <ButtonText>Continue</ButtonText>
            </Button>
        </View>
    )
}
