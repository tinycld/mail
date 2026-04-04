#!/usr/bin/env npx tsx
/**
 * Server API smoke test
 *
 * Authenticates against PocketBase and exercises server APIs.
 * Reads SMOKE_TEST_USER and SMOKE_TEST_PW from .env, or accepts --email/--password flags.
 *
 * Usage:
 *   npx tsx scripts/test-server-api.ts [--email <email>] [--password <pw>] [--url <url>]
 */

try {
    process.loadEnvFile()
} catch {
    // .env may not exist
}

interface Config {
    url: string
    email: string
    password: string
}

function parseArgs(): Config {
    const args = process.argv.slice(2)
    const config: Config = {
        url: process.env.SMOKE_TEST_ADDRESS || 'http://127.0.0.1:7090',
        email: process.env.SMOKE_TEST_USER || '',
        password: process.env.SMOKE_TEST_PW || '',
    }

    for (let i = 0; i < args.length; i++) {
        switch (args[i]) {
            case '--url':
                config.url = args[++i]
                break
            case '--email':
                config.email = args[++i]
                break
            case '--password':
                config.password = args[++i]
                break
            case '--help':
                console.log(
                    'Usage: npx tsx scripts/test-server-api.ts [--email <email>] [--password <pw>] [--url <url>]'
                )
                console.log('  Or set SMOKE_TEST_USER and SMOKE_TEST_PW in .env')
                process.exit(0)
        }
    }

    if (!config.email || !config.password) {
        console.error(
            'Error: credentials required via --email/--password flags or SMOKE_TEST_USER/SMOKE_TEST_PW in .env'
        )
        process.exit(1)
    }

    return config
}

let passed = 0
let failed = 0

function ok(label: string, detail?: string) {
    passed++
    console.log(`  ✓ ${label}${detail ? ` — ${detail}` : ''}`)
}

function fail(label: string, detail: string) {
    failed++
    console.error(`  ✗ ${label} — ${detail}`)
}

async function testHealth(config: Config) {
    console.log('\n▸ Health check')
    try {
        const res = await fetch(`${config.url}/api/health`)
        if (res.ok) {
            ok('GET /api/health', `${res.status}`)
        } else {
            fail('GET /api/health', `status ${res.status}`)
        }
    } catch (err) {
        fail('GET /api/health', String(err))
    }
}

async function testAuth(config: Config): Promise<string | null> {
    console.log('\n▸ Authentication')
    try {
        const res = await fetch(`${config.url}/api/collections/users/auth-with-password`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ identity: config.email, password: config.password }),
        })
        const data = await res.json()
        if (res.ok && data.token) {
            ok('POST /api/collections/users/auth-with-password', `token obtained`)
            return data.token
        }
        fail('Auth', `status ${res.status}: ${data.message || 'unknown error'}`)
        return null
    } catch (err) {
        fail('Auth', String(err))
        return null
    }
}

async function testCardDAV(config: Config) {
    console.log('\n▸ CardDAV')
    const basicAuth = Buffer.from(`${config.email}:${config.password}`).toString('base64')
    const headers = { Authorization: `Basic ${basicAuth}` }

    // PROPFIND on well-known
    try {
        const res = await fetch(`${config.url}/.well-known/carddav`, {
            method: 'GET',
            headers,
            redirect: 'manual',
        })
        if (res.status === 301 || res.status === 302 || res.status === 200) {
            ok('GET /.well-known/carddav', `${res.status} → ${res.headers.get('location') || 'ok'}`)
        } else {
            fail('GET /.well-known/carddav', `status ${res.status}`)
        }
    } catch (err) {
        fail('GET /.well-known/carddav', String(err))
    }

    // Step 1: Discover current-user-principal
    let principalPath: string | null = null
    try {
        const body = `<?xml version="1.0" encoding="utf-8"?>
<d:propfind xmlns:d="DAV:">
  <d:prop>
    <d:current-user-principal/>
  </d:prop>
</d:propfind>`

        const res = await fetch(`${config.url}/carddav/`, {
            method: 'PROPFIND',
            headers: { ...headers, 'Content-Type': 'application/xml', Depth: '0' },
            body,
        })
        const text = await res.text()
        if (res.status === 207) {
            const hrefMatch = text.match(/<[A-Za-z:]*href[^>]*>([^<]*principals[^<]*)</)
            if (hrefMatch) {
                principalPath = hrefMatch[1]
                ok('Discover principal', principalPath)
            } else {
                fail('Discover principal', `no principal href in response: ${text.slice(0, 300)}`)
            }
        } else {
            fail('PROPFIND /carddav/', `status ${res.status}: ${text.slice(0, 200)}`)
        }
    } catch (err) {
        fail('PROPFIND /carddav/', String(err))
    }

    // Step 2: List address books under the principal
    let addressBookPath: string | null = null
    if (principalPath) {
        try {
            const body = `<?xml version="1.0" encoding="utf-8"?>
<d:propfind xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav">
  <d:prop>
    <d:resourcetype/>
    <d:displayname/>
  </d:prop>
</d:propfind>`

            const res = await fetch(`${config.url}${principalPath}`, {
                method: 'PROPFIND',
                headers: { ...headers, 'Content-Type': 'application/xml', Depth: '1' },
                body,
            })
            const text = await res.text()
            if (res.status === 207) {
                // Find an href deeper than the principal itself (an address book)
                const allHrefs = [...text.matchAll(/<[A-Za-z:]*href[^>]*>([^<]+)</g)].map(m => m[1])
                addressBookPath =
                    allHrefs.find(h => h !== principalPath && h.startsWith(principalPath || '')) ||
                    null
                if (addressBookPath) {
                    ok('Discovered address book', addressBookPath)
                } else {
                    fail(
                        'Discover address book',
                        `no child address book found. hrefs: ${allHrefs.join(', ')}`
                    )
                }
            } else {
                fail(`PROPFIND ${principalPath}`, `status ${res.status}: ${text.slice(0, 200)}`)
            }
        } catch (err) {
            fail(`PROPFIND ${principalPath}`, String(err))
        }
    }

    if (!addressBookPath) {
        fail('CardDAV list/create/delete', 'skipped — no address book discovered')
        return
    }

    // List contacts via REPORT
    try {
        const body = `<?xml version="1.0" encoding="utf-8"?>
<card:addressbook-query xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav">
  <d:prop>
    <d:getetag/>
    <card:address-data/>
  </d:prop>
</card:addressbook-query>`

        const res = await fetch(`${config.url}${addressBookPath}`, {
            method: 'REPORT',
            headers: { ...headers, 'Content-Type': 'application/xml', Depth: '1' },
            body,
        })
        if (res.status === 207) {
            const text = await res.text()
            const contactCount = (text.match(/<[A-Za-z:]*response[^>]*>/g) || []).length
            ok('REPORT address-book-query', `${contactCount} contacts`)
        } else {
            fail('REPORT address-book-query', `status ${res.status}`)
        }
    } catch (err) {
        fail('REPORT address-book-query', String(err))
    }

    // PUT a new contact
    const testUID = `urn:uuid:test-api-${Date.now()}`
    const vcardPath = `${addressBookPath}${testUID}.vcf`
    try {
        const vcard = [
            'BEGIN:VCARD',
            'VERSION:3.0',
            `UID:${testUID}`,
            'FN:API Test Contact',
            'N:Contact;API Test;;;',
            'EMAIL:apitest@example.com',
            'TEL:555-999-0000',
            'END:VCARD',
        ].join('\r\n')

        const res = await fetch(`${config.url}${vcardPath}`, {
            method: 'PUT',
            headers: { ...headers, 'Content-Type': 'text/vcard' },
            body: vcard,
        })
        if (res.status >= 200 && res.status < 300) {
            ok('PUT contact', `${res.status} created ${testUID}`)
        } else {
            const text = await res.text()
            fail('PUT contact', `status ${res.status}: ${text.slice(0, 200)}`)
        }
    } catch (err) {
        fail('PUT contact', String(err))
    }

    // GET the contact back
    try {
        const res = await fetch(`${config.url}${vcardPath}`, {
            method: 'GET',
            headers,
        })
        if (res.ok) {
            const text = await res.text()
            if (text.includes('API Test Contact')) {
                ok('GET contact', 'vcard content verified')
            } else {
                fail('GET contact', 'vcard content missing expected data')
            }
        } else {
            fail('GET contact', `status ${res.status}`)
        }
    } catch (err) {
        fail('GET contact', String(err))
    }

    // DELETE the test contact
    try {
        const res = await fetch(`${config.url}${vcardPath}`, {
            method: 'DELETE',
            headers,
        })
        if (res.ok || res.status === 204) {
            ok('DELETE contact', `${res.status}`)
        } else {
            fail('DELETE contact', `status ${res.status}`)
        }
    } catch (err) {
        fail('DELETE contact', String(err))
    }
}

async function main() {
    const config = parseArgs()
    console.log(`\nTesting server at ${config.url} as ${config.email}`)

    await testHealth(config)
    await testAuth(config)
    await testCardDAV(config)

    console.log(`\n${passed} passed, ${failed} failed\n`)
    if (failed > 0) process.exit(1)
}

main()
