import { useLiveQuery } from '@tanstack/react-db'
import type { AddDomainResponse, OutboundCheckResult } from '@tinycld/app-generated/mail-api'
import { SetupContinueButton } from '@tinycld/core/components/setup/wizard/SetupContinueButton'
import { SetupTabs } from '@tinycld/core/components/setup/wizard/SetupTabs'
import { StepHeading } from '@tinycld/core/components/setup/wizard/StepHeading'
import { SlotContribution } from '@tinycld/core/components/sidebar-primitives/SidebarSlot'
import { errorToString } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { packageSidebarContributions } from '@tinycld/core/lib/packages/derive-components'
import { pb, useStore } from '@tinycld/core/lib/pocketbase'
import type { SetupStepProps } from '@tinycld/core/lib/setup/types'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useIsSettingManaged } from '@tinycld/core/lib/use-managed-settings'
import { ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import { useState } from 'react'
import { Text, View } from 'react-native'
import { AddDomainForm } from '../settings/AddDomainForm'
import { DnsRecordsPanel, hasUnpublishedDnsRecords } from '../settings/DnsRecordsPanel'
import { isDomainReady } from '../settings/domain-ready'
import { DeleteDomainButton, RemoveDomainConfirm, useRemoveDomain } from '../settings/RemoveDomain'
import { type VerificationProblem, verificationProblems } from '../settings/verification-problems'
import { assertVerifySaved } from '../settings/verify-domain'
import { useDomainChoiceStore } from '../stores/domain-choice-store'
import { currentChoice, domainTabs, showDomainTabs, showOwnDomainForm } from './domain-choice'
import {
    domainPanelTarget,
    hasReadyDomain,
    logSetupFailure,
    setupAddDomainError,
} from './setup-logic'

// Adding and verifying domains is an owner/admin action on the server.
export function useIsStepVisible() {
    const { isReady, isAdmin } = useCurrentRole()
    return isReady ? isAdmin : undefined
}

export function useIsStepDone() {
    const [domainsCollection] = useStore('mail_domains')
    const { data, isReady } = useLiveQuery({
        queryKey: ['mail_setup_domain_readiness'],
        query: query =>
            query.from({ d: domainsCollection }).select(({ d }) => ({
                verified: d.verified,
                spf_verified: d.spf_verified,
                dkim_verified: d.dkim_verified,
                return_path_verified: d.return_path_verified,
            })),
    })
    return isReady ? hasReadyDomain(data ?? []) : undefined
}

function useDomains() {
    const [domainsCollection] = useStore('mail_domains')
    const { data = [] } = useLiveQuery({
        queryKey: ['mail_setup_domains'],
        query: query =>
            query
                .from({ d: domainsCollection })
                .orderBy(({ d }) => d.created, 'asc')
                .select(({ d }) => ({
                    id: d.id,
                    domain: d.domain,
                    verified: d.verified,
                    mx_verified: d.mx_verified,
                    inbound_domain_verified: d.inbound_domain_verified,
                    spf_verified: d.spf_verified,
                    dkim_verified: d.dkim_verified,
                    return_path_verified: d.return_path_verified,
                    lastCheckedAt: d.last_checked_at,
                    details: d.verification_details,
                })),
    })
    return data.map(({ details, ...d }) => ({
        ...d,
        isReady: isDomainReady(d),
        outbound: details?.outbound,
        problems: verificationProblems(d, details),
    }))
}

type DomainItem = ReturnType<typeof useDomains>[number]

function useVerifyDomain() {
    const verify = useMutation({
        mutationFn: async (id: string) =>
            assertVerifySaved(await pb.send(`/api/mail/domains/${id}/verify`, { method: 'POST' })),
        // Shown under the Verify button; the default toast would repeat it.
        onError: error => logSetupFailure('mail.setup.verify', error),
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
    problems: VerificationProblem[]
    lastCheckedAt: string
}

// The domain the DNS + Verify panel is for. A just-added domain shows the add
// response's records until its row syncs; a row's records come from its last
// verify.
function useDomainPanel(domains: DomainItem[]) {
    const [added, setAdded] = useState<AddDomainResponse | null>(null)
    const { verify, isPending, errorMessage } = useVerifyDomain()
    const row = domainPanelTarget(domains, added?.id)
    const fromRow: PanelDomain | null = row
        ? {
              id: row.id,
              domain: row.domain,
              outbound: row.outbound,
              isVerified: row.isReady,
              problems: row.problems,
              lastCheckedAt: row.lastCheckedAt,
          }
        : null
    // Not checked yet: the add response carries the records, not a verify.
    const fromAdded: PanelDomain | null = added
        ? {
              id: added.id,
              domain: added.domain,
              outbound: added.records,
              isVerified: false,
              problems: [],
              lastCheckedAt: '',
          }
        : null
    const domain = fromRow ?? fromAdded
    return {
        domain,
        setAdded,
        // A removed domain must not live on in the panel through the add
        // response the panel falls back to before the row syncs.
        onDomainRemoved: (id: string) => {
            if (added?.id === id) setAdded(null)
        },
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

function ProblemLine({ problem }: { problem: VerificationProblem }) {
    return (
        <Text className="text-sm text-foreground">
            <Text className="font-semibold">{problem.label}: </Text>
            {problem.detail}
        </Text>
    )
}

// After a Verify that did not pass, say which checks failed and what usually
// fixes them, so the person is not left with an unchanged "Not verified".
function VerifyProblems({ domain, isVisible }: { domain: PanelDomain; isVisible: boolean }) {
    if (!isVisible || domain.isVerified || domain.problems.length === 0) return null
    const checkedAt = domain.lastCheckedAt
        ? new Date(domain.lastCheckedAt).toLocaleTimeString([], {
              hour: 'numeric',
              minute: '2-digit',
          })
        : null
    const lines = domain.problems.map(p => <ProblemLine key={p.label} problem={p} />)
    return (
        <View
            testID="setup-domain-problems"
            className="gap-1.5 rounded-lg border border-warning bg-warning/10 p-3"
        >
            <Text className="text-sm font-semibold text-foreground">
                {checkedAt ? `The check at ${checkedAt} found:` : 'The last check found:'}
            </Text>
            {lines}
            <Text className="mt-1 text-xs text-muted-foreground">{VERIFY_HELP}</Text>
        </View>
    )
}

const VERIFY_HELP =
    'DNS changes can take from a few minutes to a few hours to show. Check that each record ' +
    'matches exactly. Many DNS providers add your domain to the host name for you, so enter ' +
    'only the part before your domain. Then select Verify again.'

function DomainPanel({
    domain,
    onVerify,
    isPending,
    errorMessage,
}: Omit<ReturnType<typeof useDomainPanel>, 'setAdded' | 'onDomainRemoved'>) {
    if (!domain) return null
    return (
        <View testID="setup-new-domain" className="mb-4 gap-2 rounded-xl border border-border p-3">
            <PanelIntro domain={domain} />
            <DnsRecordsPanel outbound={domain.outbound} isVisible />
            <View className="flex-row items-center gap-3">
                <ServerActionButton
                    variant="outline"
                    className="self-start"
                    onPress={onVerify}
                    isDisabled={isPending}
                >
                    <ButtonText>{isPending ? 'Verifying…' : 'Verify'}</ButtonText>
                </ServerActionButton>
                <VerifiedLabel isVerified={domain.isVerified} />
            </View>
            <ErrorText message={errorMessage} />
            <VerifyProblems domain={domain} isVisible={!isPending} />
        </View>
    )
}

// A mistyped domain can never verify, so every row can be removed here rather
// than only from Settings after the wizard.
function DomainRow({ domain, onRemoved }: { domain: DomainItem; onRemoved: (id: string) => void }) {
    const removal = useRemoveDomain(domain.id, () => onRemoved(domain.id))
    return (
        <View className="gap-2 border-b border-border py-2">
            <View className="flex-row items-center justify-between gap-3">
                <Text className="flex-1 text-sm text-foreground">{domain.domain}</Text>
                <VerifiedLabel isVerified={domain.isReady} />
                <DeleteDomainButton isVisible={!removal.isConfirming} onPress={removal.onStart} />
            </View>
            <RemoveDomainConfirm
                isVisible={removal.isConfirming}
                domain={domain.domain}
                onConfirm={removal.onConfirm}
                onCancel={removal.onCancel}
            />
        </View>
    )
}

function DomainList({
    domains,
    onRemoved,
}: {
    domains: DomainItem[]
    onRemoved: (id: string) => void
}) {
    if (domains.length === 0) return null
    const rows = domains.map(d => <DomainRow key={d.id} domain={d} onRemoved={onRemoved} />)
    return (
        <View className="mb-4">
            <Text className="text-sm font-semibold text-foreground">Your domains</Text>
            {rows}
        </View>
    )
}

// One tab per contributed domain option, then the own-domain form. Contributed
// panels run their own setup: slot components get no props, so they cannot
// report back, and the choice only decides which panel shows.
function useDomainChoice() {
    const contributed = packageSidebarContributions.mail?.['setup-domain-options'] ?? []
    const tabs = domainTabs(contributed)
    const picked = useDomainChoiceStore(s => s.choice)
    const setChoice = useDomainChoiceStore(s => s.setChoice)
    const choice = currentChoice(tabs, picked)
    return {
        tabs,
        choice,
        setChoice,
        hasTabs: showDomainTabs(tabs),
        isOwnSelected: showOwnDomainForm(choice),
        contributedPanel: contributed.find(c => c.contributorSlug === choice) ?? null,
    }
}

type DomainChoiceState = ReturnType<typeof useDomainChoice>

function DomainTabBar({ state }: { state: DomainChoiceState }) {
    if (!state.hasTabs) return null
    return (
        <SetupTabs
            tabs={state.tabs}
            value={state.choice}
            onChange={state.setChoice}
            testID="setup-domain-tab"
        />
    )
}

// The selected contribution's panel, loaded on first show like any slot.
function ContributedPanel({ entry }: { entry: DomainChoiceState['contributedPanel'] }) {
    if (!entry) return null
    return <SlotContribution entry={entry} />
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
        <View className="gap-1">
            <Text className="text-sm text-muted-foreground">
                Use a domain you already own, like yourcompany.com.
            </Text>
            <AddDomainForm onAdded={setAdded} describeError={describeAddError} />
            <DomainPanel {...panelProps} />
        </View>
    )
}

export default function EmailDomainStep({ next }: SetupStepProps) {
    const domains = useDomains()
    const panel = useDomainPanel(domains)
    const choice = useDomainChoice()
    // Continue waits for a domain that can send and receive, so a press meant
    // for Add cannot leave the step with nothing set up. Skip stays available.
    const hasUsableDomain = domains.some(d => d.isReady)
    return (
        <View>
            <StepHeading
                title="Your email domain"
                lead="Where should your team's email addresses live?"
                helpTopic="mail:custom-domains"
            />
            <DomainTabBar state={choice} />
            <ContributedPanel entry={choice.contributedPanel} />
            <OwnDomainForm isVisible={choice.isOwnSelected} panel={panel} />
            <DomainList domains={domains} onRemoved={panel.onDomainRemoved} />
            <SetupContinueButton onPress={next} isDisabled={!hasUsableDomain} />
        </View>
    )
}
