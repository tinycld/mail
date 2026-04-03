/**
 * Playwright Global Setup
 *
 * Resets and seeds a dedicated test database, then keeps PocketBase
 * running on port 7091 for the duration of the test run.
 * Uses server/pb_test_data so tests never interfere with dev.
 */

import { spawn, spawnSync } from 'node:child_process'
import * as fs from 'node:fs'
import * as path from 'node:path'

const PROJECT_ROOT = path.resolve(import.meta.dirname, '..')
const PB_BINARY = path.join(PROJECT_ROOT, 'server/tinycld')
const PB_DATA_DIR = path.join(PROJECT_ROOT, 'server/pb_test_data')
const PB_MIGRATIONS_DIR = path.join(PROJECT_ROOT, 'server/pb_migrations')
const PB_PORT = 7091
const PID_FILE = path.join(PROJECT_ROOT, 'server/.test-pb.pid')

async function waitForPocketBase(maxAttempts = 30): Promise<boolean> {
    for (let i = 0; i < maxAttempts; i++) {
        try {
            const response = await fetch(`http://127.0.0.1:${PB_PORT}/api/health`)
            if (response.ok) return true
        } catch {
            // not ready yet
        }
        await new Promise(r => setTimeout(r, 1000))
    }
    return false
}

export default async function globalSetup() {
    // biome-ignore lint/suspicious/noConsole: test setup status messages
    console.log('\n[test-setup] Resetting test database...')

    // reset-dev-db.ts kills any existing process on PB_PORT before starting
    const result = spawnSync(
        'npx',
        [
            'tsx',
            'scripts/reset-dev-db.ts',
            '--url',
            `http://127.0.0.1:${PB_PORT}`,
            '--data-dir',
            'server/pb_test_data',
        ],
        {
            cwd: PROJECT_ROOT,
            stdio: 'inherit',
            env: process.env,
            timeout: 60_000,
        }
    )

    if (result.status !== 0) {
        throw new Error(`[test-setup] Database reset failed with code ${result.status}`)
    }

    // biome-ignore lint/suspicious/noConsole: test setup status messages
    console.log('[test-setup] Starting PocketBase on port', PB_PORT)
    const pb = spawn(
        PB_BINARY,
        [
            '--dev',
            '--dir',
            PB_DATA_DIR,
            '--migrationsDir',
            PB_MIGRATIONS_DIR,
            '--http',
            `127.0.0.1:${PB_PORT}`,
            'serve',
        ],
        {
            stdio: 'ignore',
            detached: true,
        }
    )
    pb.unref()

    const ready = await waitForPocketBase()
    if (!ready) {
        pb.kill()
        throw new Error('[test-setup] PocketBase failed to start')
    }

    // Write PID file so teardown can reliably stop it
    fs.writeFileSync(PID_FILE, String(pb.pid))

    // biome-ignore lint/suspicious/noConsole: test setup status messages
    console.log('[test-setup] Test database ready, PocketBase running.\n')
}
