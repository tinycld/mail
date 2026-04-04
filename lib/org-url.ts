const PROD_BASE_DOMAIN = 'tinycld.com'

function getBaseDomain(): string {
    if (process.env.VITE_BASE_DOMAIN) return process.env.VITE_BASE_DOMAIN
    if (process.env.NODE_ENV === 'production') return PROD_BASE_DOMAIN

    // In dev, derive from the current window location so the port is always correct
    if (typeof window !== 'undefined') {
        const { hostname, port } = window.location
        let base = hostname
        if (hostname.endsWith('.localhost')) {
            base = 'localhost'
        }
        return port ? `${base}:${port}` : base
    }

    return 'localhost:7100'
}

function getProtocol(): string {
    if (typeof window !== 'undefined') return window.location.protocol.replace(':', '')
    if (process.env.NODE_ENV === 'production') return 'https'
    return 'http'
}

export function getOrgUrl(orgSlug: string): string {
    const base = getBaseDomain()
    const protocol = getProtocol()
    return `${protocol}://${orgSlug}.${base}`
}

export function navigateToOrg(orgSlug: string): void {
    if (typeof window !== 'undefined') {
        window.location.href = `${getOrgUrl(orgSlug)}/app`
    }
}
