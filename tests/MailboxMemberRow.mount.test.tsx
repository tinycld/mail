// @vitest-environment happy-dom
import { cleanup, render } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { MailboxMemberRow } from '~/tinycld/mail/settings/MailboxMemberRow'

afterEach(cleanup)

vi.mock('@tinycld/core/lib/use-app-theme', () => ({ useThemeColor: () => '#000000' }))
vi.mock('@tinycld/core/lib/use-writes-available', () => ({
    useServerActionState: ({ isDisabled }: { isDisabled: boolean }) => ({
        isDisabled,
        accessibilityHint: '',
    }),
}))

const props = {
    name: 'Ann',
    role: 'member' as const,
    canRemove: true,
    canToggleRole: true,
    onToggleRole: () => {},
    onRemove: () => {},
}

test('an owner of the inbox sees the remove control', () => {
    const { container } = render(<MailboxMemberRow {...props} canEdit />)
    expect(container.querySelector('[accessibilitylabel="Remove Ann"]')).not.toBeNull()
})

test('an admin who does not own the inbox sees no remove control', () => {
    const { container } = render(<MailboxMemberRow {...props} canEdit={false} />)
    expect(container.querySelector('[accessibilitylabel="Remove Ann"]')).toBeNull()
})
