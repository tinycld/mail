const FOLDER_TITLES: Record<string, string> = {
    'all-inboxes': 'All Inboxes',
}

export function stripHtmlTags(html: string): string {
    return html.replace(/<[^>]*>/g, '')
}

export function prettifyFolderKey(key: string): string {
    if (FOLDER_TITLES[key]) return FOLDER_TITLES[key]
    return key
        .split('-')
        .map(part => (part ? part.charAt(0).toUpperCase() + part.slice(1) : part))
        .join(' ')
}
