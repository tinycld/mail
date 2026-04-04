const DEV_PB_ADDR = 'http://127.0.0.1:7090'

function resolveServerAddr(): string {
    if (process.env.NODE_ENV !== 'production') return DEV_PB_ADDR
    if (typeof window === 'undefined') return DEV_PB_ADDR

    // In production, PB serves the app — strip the org subdomain from origin
    const { protocol, hostname, port } = window.location
    let baseHost = hostname
    const parts = hostname.split('.')
    if (parts.length >= 3) {
        baseHost = parts.slice(1).join('.')
    }

    return `${protocol}//${baseHost}${port ? `:${port}` : ''}`
}

export const PB_SERVER_ADDR =
    process.env.VITE_PB_SERVER_ADDR || process.env.PB_SERVER_ADDR || resolveServerAddr()
