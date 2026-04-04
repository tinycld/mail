import { parseSubdomain } from '~/lib/hostname'

const DEV_PB_ADDR = 'http://127.0.0.1:7090'

function resolveServerAddr(): string {
    if (process.env.NODE_ENV !== 'production') return DEV_PB_ADDR
    if (typeof window === 'undefined') return DEV_PB_ADDR

    const { protocol, hostname, port } = window.location
    const { baseDomain } = parseSubdomain(hostname)
    return `${protocol}//${baseDomain}${port ? `:${port}` : ''}`
}

export const PB_SERVER_ADDR =
    process.env.VITE_PB_SERVER_ADDR || process.env.PB_SERVER_ADDR || resolveServerAddr()
