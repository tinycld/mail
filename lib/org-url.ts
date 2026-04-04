import { parseSubdomain } from '~/lib/hostname'

const PROD_BASE_DOMAIN = 'tinycld.com'

function resolveBaseDomain(): string {
    if (process.env.VITE_BASE_DOMAIN) return process.env.VITE_BASE_DOMAIN
    if (process.env.NODE_ENV === 'production') return PROD_BASE_DOMAIN

    if (typeof window !== 'undefined') {
        const { hostname, port } = window.location
        const { baseDomain } = parseSubdomain(hostname)
        return port ? `${baseDomain}:${port}` : baseDomain
    }

    return 'localhost:7100'
}

function resolveProtocol(): string {
    if (typeof window !== 'undefined') return window.location.protocol.replace(':', '')
    if (process.env.NODE_ENV === 'production') return 'https'
    return 'http'
}

const BASE_DOMAIN = resolveBaseDomain()
const PROTOCOL = resolveProtocol()

export function getOrgUrl(orgSlug: string): string {
    return `${PROTOCOL}://${orgSlug}.${BASE_DOMAIN}`
}

export function navigateToOrg(orgSlug: string): void {
    if (typeof window !== 'undefined') {
        window.location.href = `${getOrgUrl(orgSlug)}/app`
    }
}
