import { expect, it } from 'vitest'
import { inboxAccessLines } from '~/tinycld/mail/settings/shared-inbox-access'

const row = (groupId: string, groupName: string) => ({
    mailboxId: 'mb',
    label: 'Support',
    groupId,
    groupName,
})

it('a derived row with a blank group name is not direct', () => {
    expect(inboxAccessLines([row('g1', '')])[0].text).toBe('Support · through a group')
})

it('two groups with the same name stay two entries', () => {
    expect(inboxAccessLines([row('g1', 'Sales'), row('g2', 'Sales')])[0].text).toBe(
        'Support · through Sales, Sales'
    )
})

it('the same group twice is listed once', () => {
    expect(inboxAccessLines([row('g1', 'Sales'), row('g1', 'Sales')])[0].text).toBe(
        'Support · through Sales'
    )
})

it('a direct row hides the groups', () => {
    expect(inboxAccessLines([row('g1', 'Sales'), row('', '')])[0].text).toBe('Support')
})
