import type { MailThreadState } from '../types'

// Files a thread state under a folder. The Sent view reads the is_sent flag,
// not the folder (a reply leaves its thread in the Inbox), so moving a
// thread to Sent must also set the flag or it would vanish from every view
// but All Mail.
export function moveThreadStateToFolder(
    draft: Pick<MailThreadState, 'folder' | 'is_sent'>,
    folder: MailThreadState['folder']
) {
    draft.folder = folder
    if (folder === 'sent') draft.is_sent = true
}
