import type { PackageNotifyEvents } from '@tinycld/core/lib/notify/registry'

declare module '@tinycld/core/lib/notify/events' {
    interface NotificationEvents {
        'mail.send_failed': { error: string }
        'mail.send_blocked_warn': { reason: string }
        'mail.send_blocked_error': { reason: string }
        'mail.attachments_rejected': { reason: string }
    }
}

/** Policy for mail's user-facing notifications; core composes these at boot. */
const notifyEvents = {
    'mail.send_failed': { channels: ['toast'], variant: 'error' },
    'mail.send_blocked_warn': { channels: ['toast'], variant: 'warning' },
    'mail.send_blocked_error': { channels: ['toast'], variant: 'error' },
    'mail.attachments_rejected': { channels: ['toast'], variant: 'error' },
} satisfies PackageNotifyEvents

export default notifyEvents
