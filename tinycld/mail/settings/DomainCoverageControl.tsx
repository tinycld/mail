import { and, eq, inArray, not } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import {
    type Control,
    FormErrorSummary,
    MultiSelectInput,
    RadioInput,
    useForm,
    useWatch,
    z,
    zodResolver,
} from '@tinycld/core/ui/form'
import { useMemo } from 'react'
import { Text, View } from 'react-native'
import { type Coverage, coverageChangePreview, coveragePreviewText } from './coverage-preview'

const coverageSchema = z
    .object({
        coverage: z.enum(['everyone', 'groups', 'manual']),
        coverage_groups: z.array(z.string()),
    })
    .superRefine((v, ctx) => {
        if (v.coverage === 'groups' && v.coverage_groups.length === 0) {
            ctx.addIssue({
                code: 'custom',
                path: ['coverage_groups'],
                message: 'Choose at least one group',
            })
        }
    })

type CoverageValues = z.infer<typeof coverageSchema>

const OPTIONS = [
    {
        value: 'everyone',
        label: 'Everyone',
        description: 'Each person gets their email name on this domain.',
    },
    {
        value: 'groups',
        label: 'Members of these groups',
        description: 'Only members of the groups you choose get their email name on this domain.',
    },
    { value: 'manual', label: 'Only people I add', description: 'Add each address by hand.' },
]

// One row per (person, membership in a chosen group), or one row with no
// group for a person in none of them: fold them into the preview's inputs.
function foldPeople(
    rows: { userId: string; emailName: string; inbox: string; groupId?: string }[]
) {
    const people = new Map<string, { userId: string; emailName: string; inbox: string }>()
    const memberships = new Map<string, string[]>()
    for (const { groupId, ...person } of rows) {
        people.set(person.userId, person)
        if (!groupId) continue
        memberships.set(person.userId, [...(memberships.get(person.userId) ?? []), groupId])
    }
    return { people: [...people.values()], memberships }
}

function useCoveragePreview(
    domainId: string,
    domainName: string,
    target: Coverage,
    targetGroups: string[]
) {
    const [identitiesCollection, usersCollection, addressesCollection, groupMembersCollection] =
        useStore('mail_identities', 'users', 'mail_addresses', 'group_members')
    // Memberships matter only to a groups rule. With no group chosen the
    // filter matches nothing, and pbtsdb sends no request for it.
    const groupIds = target === 'groups' ? targetGroups : []
    const { data: rows } = useLiveQuery(query => {
        const memberships = query
            .from({ member: groupMembersCollection })
            .where(({ member }) => inArray(member.group, groupIds))
        return (
            query
                .from({ identity: identitiesCollection })
                .innerJoin({ user: usersCollection }, ({ identity, user }) =>
                    eq(identity.user, user.id)
                )
                .join({ member: memberships }, ({ identity, member }) =>
                    eq(identity.user, member.user)
                )
                // A retired identity belongs to an offboarded person: it has no
                // inbox and a tombstone email name, so it never gets an address.
                .where(({ user, identity }) =>
                    and(not(eq(user.role, 'guest')), eq(identity.retired, false))
                )
                .select(({ identity, member }) => ({
                    userId: identity.user,
                    emailName: identity.email_name,
                    inbox: identity.inbox,
                    groupId: member?.group,
                }))
        )
    })
    const { people, memberships } = useMemo(() => foldPeople(rows ?? []), [rows])
    const { data: addressesOnDomain } = useLiveQuery(query =>
        query
            .from({ address: addressesCollection })
            .where(({ address }) => eq(address.domain, domainId))
            .select(({ address }) => ({
                localPart: address.local_part,
                mailbox: address.mailbox,
                source: address.source,
            }))
    )
    return coverageChangePreview({
        target,
        targetGroups: groupIds,
        domainName,
        people,
        memberships,
        addressesOnDomain: addressesOnDomain ?? [],
    })
}

function useGroupOptions() {
    const [groupsCollection] = useStore('groups')
    const { data } = useLiveQuery(query =>
        query
            .from({ group: groupsCollection })
            .orderBy(({ group }) => group.name, 'asc')
            .select(({ group }) => ({ value: group.id, label: group.name }))
    )
    return data ?? []
}

// Order-free: the saved order and the picker's order can differ.
const sameGroups = (a: string[], b: string[]) => [...a].sort().join(',') === [...b].sort().join(',')

export function DomainCoverageControl({
    domainId,
    domainName,
    coverage,
    coverageGroups,
}: {
    domainId: string
    domainName: string
    coverage: Coverage
    coverageGroups: string[]
}) {
    const [domainsCollection] = useStore('mail_domains')
    const groupOptions = useGroupOptions()
    const {
        control,
        handleSubmit,
        reset,
        setError,
        getValues,
        formState: { errors, isSubmitted },
    } = useForm({
        mode: 'onChange',
        resolver: zodResolver(coverageSchema),
        // `values`, not defaultValues: when the domain row changes (this
        // Apply, or another admin), the form follows the saved rule.
        values: { coverage, coverage_groups: coverageGroups },
    })
    const target = useWatch({ control, name: 'coverage' })
    const targetGroups = useWatch({ control, name: 'coverage_groups' })
    const preview = useCoveragePreview(domainId, domainName, target, targetGroups)
    const isChanged = target !== coverage || !sameGroups(targetGroups, coverageGroups)

    const save = useMutation({
        mutationFn: mutation(function* (data: CoverageValues) {
            yield domainsCollection.update(domainId, d => {
                d.coverage = data.coverage
                d.coverage_groups = data.coverage_groups
            })
        }),
        onSuccess: () => reset(getValues()),
        onError: handleMutationErrorsWithForm({ setError, getValues }),
    })
    const onConfirm = handleSubmit(data => save.mutate(data))

    return (
        <View className="gap-2">
            <RadioInput
                control={control}
                name="coverage"
                label="Who gets an address here?"
                options={OPTIONS}
            />
            <GroupPicker isVisible={target === 'groups'} control={control} options={groupOptions} />
            <FormErrorSummary errors={errors} isEnabled={isSubmitted} />
            <ConfirmCoverage
                isVisible={isChanged}
                text={coveragePreviewText(preview)}
                isPending={save.isPending}
                onConfirm={onConfirm}
                onCancel={() => reset({ coverage, coverage_groups: coverageGroups })}
            />
        </View>
    )
}

function GroupPicker({
    isVisible,
    control,
    options,
}: {
    isVisible: boolean
    control: Control<CoverageValues>
    options: { value: string; label: string }[]
}) {
    if (!isVisible) return null
    return (
        <MultiSelectInput
            control={control}
            name="coverage_groups"
            label="Groups"
            options={options}
            emptyText="No groups yet. Create one in Settings → Groups."
        />
    )
}

function ConfirmCoverage({
    isVisible,
    text,
    isPending,
    onConfirm,
    onCancel,
}: {
    isVisible: boolean
    text: string
    isPending: boolean
    onConfirm: () => void
    onCancel: () => void
}) {
    if (!isVisible) return null
    return (
        <View className="gap-2 rounded-lg border border-border p-2">
            <Text className="text-foreground" style={{ fontSize: 13 }}>
                {text}
            </Text>
            <View className="flex-row gap-2">
                <ServerActionButton size="sm" onPress={onConfirm} isDisabled={isPending}>
                    <ButtonText>Apply</ButtonText>
                </ServerActionButton>
                <ServerActionButton size="sm" variant="outline" onPress={onCancel}>
                    <ButtonText>Cancel</ButtonText>
                </ServerActionButton>
            </View>
        </View>
    )
}
