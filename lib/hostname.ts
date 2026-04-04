/**
 * Parses a hostname into its org subdomain and base domain.
 * e.g. "acme.localhost" → { slug: "acme", baseDomain: "localhost" }
 * e.g. "acme.tinycld.com" → { slug: "acme", baseDomain: "tinycld.com" }
 * e.g. "localhost" → { slug: "", baseDomain: "localhost" }
 */
export function parseSubdomain(hostname: string): { slug: string; baseDomain: string } {
    if (hostname.endsWith('.localhost')) {
        return {
            slug: hostname.slice(0, -'.localhost'.length),
            baseDomain: 'localhost',
        }
    }

    const parts = hostname.split('.')
    if (parts.length >= 3) {
        return {
            slug: parts[0],
            baseDomain: parts.slice(1).join('.'),
        }
    }

    return { slug: '', baseDomain: hostname }
}
