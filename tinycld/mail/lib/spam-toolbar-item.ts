import type { ToolbarItem } from '@tinycld/core/components/ResponsiveToolbar'
import { CircleAlert, ShieldCheck } from 'lucide-react-native'
import type { MailThreadState } from '../types'

/**
 * Report spam, or Not spam for mail already in Spam. Mail reaches Spam only
 * by a user's report or a rule, so undoing it is a move back to Inbox.
 */
export function spamToolbarItem(
    isSpam: boolean,
    onSpam: () => void,
    onMove: (folder: MailThreadState['folder']) => void
): ToolbarItem {
    if (isSpam) {
        return {
            type: 'button',
            key: 'not-spam',
            icon: ShieldCheck,
            label: 'Not spam',
            onPress: () => onMove('inbox'),
        }
    }
    return { type: 'button', key: 'spam', icon: CircleAlert, label: 'Report spam', onPress: onSpam }
}
