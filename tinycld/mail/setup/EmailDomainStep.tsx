import { useLiveQuery } from '@tanstack/react-db'
import type { AddDomainResponse, OutboundCheckResult } from '@tinycld/app-generated/mail-api'
import { HelpIcon } from '@tinycld/core/components/help/HelpIcon'
import { SetupContinueButton } from '@tinycld/core/components/setup/wizard/SetupContinueButton'
import { SidebarSlot } from '@tinycld/core/components/sidebar-primitives/SidebarSlot'
import { errorToString } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { packageSidebarContributions } from '@tinycld/core/lib/packages/derive-components'
import { pb, useStore } from '@tinycld/core/lib/pocketbase'
import type { SetupStepProps } from '@tinycld/core/lib/setup/types'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useIsSettingManaged } from '@tinycld/core/lib/use-managed-settings'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { useState } from 'react'
import { Pressable, Text, View } from 'react-native'
import { AddDomainForm } from '../settings/AddDomainForm'
import { DnsRecordsPanel, hasUnpublishedDnsRecords } from '../settings/DnsRecordsPanel'
import { assertVerifySaved } from '../settings/verify-domain'
import { type DomainChoice, initialChoice, showOwnDomainForm } from './domain-choice'
import { domainPanelTarget, hasVerifiedDomain, setupAddDomainError } from './setup-logic'

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

interface PanelDomain {
    id: string
    domain: string
    outbound: OutboundCheckResult | undefined
    isVerified: boolean
}

// The domain the DNS + Verify panel is for. A just-added domain shows the add
// response's records until its row syncs; a row's records come from its last
// verify.
function useDomainPanel(domains: DomainItem[]) {
    const [added, setAdded] = useState<AddDomainResponse | null>(null)
    const { verify, isPending, errorMessage } = useVerifyDomain()
    const row = domainPanelTarget(domains, added?.id)
    const fromRow: PanelDomain | null = row
        ? { id: row.id, domain: row.domain, outbound: row.outbound, isVerified: row.verified }
        : null
    const fromAdded: PanelDomain | null = added
        ? { id: added.id, domain: added.domain, outbound: added.records, isVerified: false }
        : null
    const domain = fromRow ?? fromAdded
    return {
        domain,
        setAdded,
        onVerify: () => {
            if (domain) verify(domain.id)
        },
        isPending,
        errorMessage,
    }
}

function VerifiedLabel({ isVerified }: { isVerified: boolean }) {
    if (isVerified) return <Text className="text-xs text-success">Verified</Text>
    return <Text className="text-xs text-muted-foreground">Not verified</Text>
}

function ErrorText({ message }: { message: string | null }) {
    if (!message) return null
    return <Text className="text-sm text-danger">{message}</Text>
}

// The publish instruction only applies while a record is still unverified; a
// domain whose DNS someone else manages has nothing for the person to publish.
function PanelIntro({ domain }: { domain: PanelDomain }) {
    if (!hasUnpublishedDnsRecords(domain.outbound)) {
        return (
            <Text className="text-sm text-foreground">
                Verify {domain.domain} to check its setup.
            </Text>
        )
    }
    return (
        <Text className="text-sm text-foreground">
            Publish the DNS records for {domain.domain} at your DNS provider, then verify. DNS
            changes can take some time to show.
        </Text>
    )
}

function DomainPanel({
    domain,
    onVerify,
    isPending,
    errorMessage,
}: Omit<ReturnType<typeof useDomainPanel>, 'setAdded'>) {
    if (!domain) return null
    return (
        <View testID="setup-new-domain" className="mb-4 gap-2 rounded-xl border border-border p-3">
            <PanelIntro domain={domain} />
            <DnsRecordsPanel outbound={domain.outbound} isVisible />
            <View className="flex-row items-center gap-3">
                <Button
                    variant="outline"
                    className="self-start"
                    onPress={onVerify}
                    isDisabled={isPending}
                >
                    <ButtonText>{isPending ? 'Verifying…' : 'Verify'}</ButtonText>
                </Button>
                <VerifiedLabel isVerified={domain.isVerified} />
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

// The choice only opens the own-domain form. Contributed option cards run
// their own setup: slot components get no props, so they cannot report back.
function useDomainChoice() {
    const contributed = packageSidebarContributions.mail?.['setup-domain-options'] ?? []
    const [choice, setChoice] = useState<DomainChoice>(() => initialChoice(contributed.length > 0))
    return {
        isOwnSelected: showOwnDomainForm(choice),
        chooseOwn: () => setChoice('own'),
    }
}

function ownCardClassName(isSelected: boolean) {
    const border = isSelected ? 'border-primary' : 'border-border'
    return `mt-2 gap-1 rounded-xl border p-3 ${border}`
}

function OwnDomainCard({ isSelected, onPress }: { isSelected: boolean; onPress: () => void }) {
    return (
        <Pressable
            testID="setup-own-domain-option"
            accessibilityRole="button"
            accessibilityState={{ selected: isSelected }}
            onPress={onPress}
            className={ownCardClassName(isSelected)}
        >
            <Text className="text-sm font-semibold text-foreground">Your own domain</Text>
            <Text className="text-sm text-muted-foreground">
                Use a domain you already own, like yourcompany.com.
            </Text>
        </Pressable>
    )
}

function OwnDomainForm({
    isVisible,
    panel,
}: {
    isVisible: boolean
    panel: ReturnType<typeof useDomainPanel>
}) {
    const isMailManaged = useIsSettingManaged('mail.')
    if (!isVisible) return null
    const { setAdded, ...panelProps } = panel
    const describeAddError = (error: unknown) => setupAddDomainError(error, isMailManaged)
    return (
        <View className="mt-2 gap-1">
            <AddDomainForm onAdded={setAdded} describeError={describeAddError} />
            <DomainPanel {...panelProps} />
        </View>
    )
}

export default function EmailDomainStep({ next }: SetupStepProps) {
    const domains = useDomains()
    const panel = useDomainPanel(domains)
    const { isOwnSelected, chooseOwn } = useDomainChoice()
    return (
        <View className="max-w-[440px] gap-1">
            <View className="flex-row items-center gap-2">
                <Text className="text-2xl font-bold text-foreground">Your email domain</Text>
                <HelpIcon topic="mail:custom-domains" />
            </View>
            <Text className="mb-3 text-sm text-muted-foreground">
                Where should your team's email addresses live?
            </Text>
            <SidebarSlot target="mail" slot="setup-domain-options" />
            <OwnDomainCard isSelected={isOwnSelected} onPress={chooseOwn} />
            <OwnDomainForm isVisible={isOwnSelected} panel={panel} />
            <DomainList domains={domains} />
            <SetupContinueButton onPress={next} />
        </View>
    )
}
