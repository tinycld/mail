export function formatBytes(bytes: number): string {
    if (bytes === 0) return '—'
    const units = ['B', 'KB', 'MB', 'GB']
    const i = Math.floor(Math.log(bytes) / Math.log(1024))
    const value = bytes / 1024 ** i
    return `${value.toFixed(i > 0 ? 1 : 0)} ${units[i]}`
}

export function formatDate(isoDate: string): string {
    const date = new Date(isoDate)
    return date.toLocaleDateString('en-US', {
        month: 'short',
        day: 'numeric',
        year: 'numeric',
    })
}
