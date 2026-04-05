import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { ArrowLeft } from 'lucide-react-native'
import { useParams, useRouter } from 'one'
import { newRecordId } from 'pbtsdb'
import { Pressable } from 'react-native'
import { Button, ScrollView, SizableText, useTheme, XStack, YStack } from 'tamagui'
import { useBreakpoint } from '~/components/workspace/useBreakpoint'
import { handleMutationErrorsWithForm } from '~/lib/errors'
import { useMutation } from '~/lib/mutations'
import { useStore } from '~/lib/pocketbase'
import { useCurrentUserOrg } from '~/lib/use-current-user-org'
import { useOrgInfo } from '~/lib/use-org-info'
import { useForm, z, zodResolver } from '~/ui/form'
import { EventForm } from '../components/EventForm'
import { EventGuestList } from '../components/EventGuestList'
import { useVisibleCalendars } from '../hooks/useCalendarEvents'
import { parseEventId } from '../lib/recurrence'

const eventSchema = z.object({
    title: z.string().min(1, 'Title is required'),
    description: z.string(),
    location: z.string(),
    startDate: z.string().regex(/^\d{4}-\d{2}-\d{2}$/, 'Use YYYY-MM-DD format'),
    startTime: z.string().regex(/^\d{2}:\d{2}$/, 'Use HH:MM format'),
    endDate: z.string().regex(/^\d{4}-\d{2}-\d{2}$/, 'Use YYYY-MM-DD format'),
    endTime: z.string().regex(/^\d{2}:\d{2}$/, 'Use HH:MM format'),
    all_day: z.boolean(),
    recurrence: z.string(),
    calendar: z.string(),
    busy_status: z.enum(['busy', 'free']),
    visibility: z.enum(['default', 'public', 'private']),
    reminderMinutes: z.number(),
})

function combineDateAndTime(dateStr: string, timeStr: string): string {
    return new Date(`${dateStr}T${timeStr}:00`).toISOString()
}

export default function EventEditorScreen() {
    const { id } = useParams<{ id: string }>()
    const router = useRouter()
    const theme = useTheme()
    const breakpoint = useBreakpoint()
    const { orgSlug } = useOrgInfo()
    const userOrg = useCurrentUserOrg(orgSlug)
    const { calendars, mineCalendars } = useVisibleCalendars()
    const [eventsCollection] = useStore('calendar_events')

    const { baseId } = parseEventId(id ?? '')
    const isNew = !id || id === 'new'
    const lookupId = isNew ? '' : baseId

    const { data: existingEvents } = useLiveQuery(
        query => query.from({ evt: eventsCollection }).where(({ evt }) => eq(evt.id, lookupId)),
        [lookupId]
    )
    const event = existingEvents?.[0]

    const startDate = event ? new Date(event.start) : new Date()
    const endDate = event ? new Date(event.end) : new Date()

    const defaultCalendar = mineCalendars[0]?.id ?? calendars[0]?.id ?? ''

    const {
        control,
        handleSubmit,
        setError,
        getValues,
        watch,
        formState: { errors, isSubmitted },
    } = useForm({
        mode: 'onChange',
        resolver: zodResolver(eventSchema),
        values: event
            ? {
                  title: event.title,
                  description: event.description,
                  location: event.location,
                  startDate: startDate.toISOString().split('T')[0],
                  startTime: startDate.toTimeString().slice(0, 5),
                  endDate: endDate.toISOString().split('T')[0],
                  endTime: endDate.toTimeString().slice(0, 5),
                  all_day: event.all_day,
                  recurrence: event.recurrence,
                  calendar: event.calendar,
                  busy_status: event.busy_status,
                  visibility: event.visibility,
                  reminderMinutes: event.reminder,
              }
            : undefined,
        defaultValues: {
            title: '',
            description: '',
            location: '',
            startDate: startDate.toISOString().split('T')[0],
            startTime: startDate.toTimeString().slice(0, 5),
            endDate: endDate.toISOString().split('T')[0],
            endTime: endDate.toTimeString().slice(0, 5),
            all_day: false,
            recurrence: '',
            calendar: defaultCalendar,
            busy_status: 'busy' as const,
            visibility: 'default' as const,
            reminderMinutes: 30,
        },
    })

    const startDateValue = watch('startDate')

    const createEvent = useMutation({
        mutationFn: function* (data: z.infer<typeof eventSchema>) {
            if (!userOrg) throw new Error('No organization context')
            yield eventsCollection.insert({
                id: newRecordId(),
                calendar: data.calendar,
                created_by: userOrg.id,
                title: data.title.trim(),
                description: data.description,
                location: data.location,
                start: combineDateAndTime(data.startDate, data.startTime),
                end: combineDateAndTime(data.endDate, data.endTime),
                all_day: data.all_day,
                recurrence: data.recurrence,
                guests: [],
                reminder: data.reminderMinutes,
                busy_status: data.busy_status,
                visibility: data.visibility,
                ical_uid: '',
            })
        },
        onSuccess: () => router.back(),
        onError: handleMutationErrorsWithForm({ setError, getValues }),
    })

    const updateEvent = useMutation({
        mutationFn: function* (data: z.infer<typeof eventSchema>) {
            yield eventsCollection.update(baseId, draft => {
                draft.title = data.title.trim()
                draft.description = data.description
                draft.location = data.location
                draft.start = combineDateAndTime(data.startDate, data.startTime)
                draft.end = combineDateAndTime(data.endDate, data.endTime)
                draft.all_day = data.all_day
                draft.recurrence = data.recurrence
                draft.calendar = data.calendar
                draft.reminder = data.reminderMinutes
                draft.busy_status = data.busy_status
                draft.visibility = data.visibility
            })
        },
        onSuccess: () => router.back(),
        onError: handleMutationErrorsWithForm({ setError, getValues }),
    })

    const isLoadingEvent = !isNew && !existingEvents
    const isNotFound = !isNew && existingEvents && !event

    if (isNotFound) {
        return (
            <YStack
                flex={1}
                alignItems="center"
                justifyContent="center"
                backgroundColor="$background"
            >
                <SizableText size="$4" color="$color8">
                    Event not found
                </SizableText>
                <Button size="$3" marginTop="$3" onPress={() => router.back()}>
                    <Button.Text>Go back</Button.Text>
                </Button>
            </YStack>
        )
    }

    const mutation = isNew ? createEvent : updateEvent
    const onSubmit = handleSubmit(data => mutation.mutate(data))
    const canSubmit = !mutation.isPending && !!userOrg && !isLoadingEvent

    const isDesktop = breakpoint === 'desktop'
    const guests = event?.guests ?? []

    const formContent = (
        <EventForm
            control={control}
            errors={errors}
            isSubmitted={isSubmitted}
            calendars={calendars}
            startDateValue={startDateValue}
        />
    )

    const guestContent = <EventGuestList guests={guests} />

    return (
        <ScrollView contentContainerStyle={{ flexGrow: 1 }} backgroundColor="$background">
            <YStack flex={1} padding="$5">
                <XStack justifyContent="space-between" alignItems="center" marginBottom="$5">
                    <XStack gap="$3" alignItems="center">
                        <Pressable onPress={() => router.back()}>
                            <ArrowLeft size={24} color={theme.color.val} />
                        </Pressable>
                        <SizableText size="$7" fontWeight="bold" color="$color">
                            {event ? 'Edit Event' : 'New Event'}
                        </SizableText>
                    </XStack>
                    <Button
                        theme="accent"
                        size="$3"
                        onPress={onSubmit}
                        disabled={!canSubmit}
                        opacity={canSubmit ? 1 : 0.5}
                    >
                        <Button.Text fontWeight="600">
                            {mutation.isPending ? 'Saving...' : 'Save'}
                        </Button.Text>
                    </Button>
                </XStack>

                {isDesktop ? (
                    <XStack gap="$5" flex={1}>
                        <YStack flex={2}>{formContent}</YStack>
                        <YStack flex={1}>{guestContent}</YStack>
                    </XStack>
                ) : (
                    <YStack gap="$4">
                        {formContent}
                        {guestContent}
                    </YStack>
                )}
            </YStack>
        </ScrollView>
    )
}
