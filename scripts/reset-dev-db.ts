#!/usr/bin/env npx tsx
/**
 * Reset Database Script
 *
 * Deletes server/pb_data, starts PocketBase to run migrations,
 * then seeds the database with a test user and org.
 *
 * Usage:
 *   npx tsx scripts/reset-dev-db.ts [options]
 *
 * Options:
 *   --url <url>        PocketBase URL (default: http://127.0.0.1:7090)
 *   --data-dir <dir>   Data directory (default: server/pb_data)
 *   --skip-build       Skip building PocketBase
 *   --keep-running     Keep server running after seeding (default: false)
 *   --help             Show this help message
 *
 * Environment variables (from .env):
 *   POCKETBASE_EMAIL     - Superuser email (or SEED_ADMIN_EMAIL)
 *   POCKETBASE_PASSWORD  - Superuser password (or SEED_ADMIN_PW)
 */

import { spawn, spawnSync } from 'node:child_process'
import * as fs from 'node:fs'
import * as path from 'node:path'

try {
    process.loadEnvFile()
} catch {
    // .env may not exist in CI
}

interface Config {
    url: string
    dataDir: string
    skipBuild: boolean
    keepRunning: boolean
}

function parseArgs(): Config {
    const args = process.argv.slice(2)
    const config: Config = {
        url: 'http://127.0.0.1:7090',
        dataDir: 'server/pb_data',
        skipBuild: false,
        keepRunning: false,
    }

    for (let i = 0; i < args.length; i++) {
        const arg = args[i]
        switch (arg) {
            case '--url':
                config.url = args[++i]
                break
            case '--data-dir':
                config.dataDir = args[++i]
                break
            case '--skip-build':
                config.skipBuild = true
                break
            case '--keep-running':
                config.keepRunning = args[++i] !== 'false'
                break
            case '--help':
                process.exit(0)
                break
            default:
                if (arg.startsWith('-')) {
                    process.exit(1)
                }
        }
    }

    return config
}

const CONFIG = parseArgs()
const PB_URL = CONFIG.url
const parsedUrl = new URL(PB_URL)
const PB_HOST = parsedUrl.hostname
const PB_PORT = parseInt(parsedUrl.port || '8090', 10)
const PB_DATA_DIR = path.join(process.cwd(), CONFIG.dataDir)
const PB_BINARY = path.join(process.cwd(), 'server/tinycld')

async function sleep(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms))
}

async function waitForPocketBase(maxAttempts = 30): Promise<boolean> {
    for (let i = 0; i < maxAttempts; i++) {
        try {
            const response = await fetch(`${PB_URL}/api/health`)
            if (response.ok) {
                return true
            }
        } catch {
            // Server not ready yet
        }
        await sleep(1000)
    }
    return false
}

function killExistingPocketBase(): void {
    try {
        const result = spawnSync('lsof', ['-ti', `:${PB_PORT}`], {
            encoding: 'utf-8',
        })
        if (result.stdout.trim()) {
            const pids = result.stdout.trim().split('\n')
            for (const pid of pids) {
                spawnSync('kill', ['-9', pid])
            }
        } else {
        }
    } catch {
        // No processes to kill
    }
}

function deletePbData(): void {
    if (fs.existsSync(PB_DATA_DIR)) {
        fs.rmSync(PB_DATA_DIR, { recursive: true, force: true })
    } else {
    }
}

function buildPocketBase(): void {
    if (CONFIG.skipBuild) {
        return
    }

    const result = spawnSync('go', ['build', '-o', 'tinycld', '.'], {
        cwd: path.join(process.cwd(), 'server'),
        stdio: 'inherit',
    })
    if (result.status !== 0) {
        throw new Error('Failed to build PocketBase')
    }
}

function getCredentials(): { email: string; password: string } {
    const email =
        process.env.POCKETBASE_EMAIL || process.env.SEED_ADMIN_EMAIL || 'admin@tinycld.org'
    const password =
        process.env.POCKETBASE_PASSWORD || process.env.SEED_ADMIN_PW || 'AdminPass1234!'
    return { email, password }
}

function createSuperuser(): void {
    const { email, password } = getCredentials()

    const result = spawnSync(
        PB_BINARY,
        ['superuser', 'upsert', email, password, '--dir', PB_DATA_DIR],
        {
            stdio: 'inherit',
        }
    )
    if (result.status !== 0) {
        throw new Error('Failed to create superuser')
    }
}

async function startPocketBase(): Promise<ReturnType<typeof spawn>> {
    const migrationsDir = path.join(process.cwd(), 'server/pb_migrations')
    const pb = spawn(
        PB_BINARY,
        [
            '--dev',
            '--dir',
            PB_DATA_DIR,
            '--migrationsDir',
            migrationsDir,
            '--http',
            `${PB_HOST}:${PB_PORT}`,
            'serve',
        ],
        {
            stdio: ['ignore', 'pipe', 'pipe'],
            detached: false,
        }
    )

    pb.stderr?.on('data', data => {
        console.error('[pocketbase]', data.toString().trim())
    })

    pb.on('error', err => {
        console.error('[pocketbase] spawn error:', err)
    })

    return pb
}

async function runSeedScript(): Promise<void> {
    const { email, password } = getCredentials()

    return new Promise((resolve, reject) => {
        const seed = spawn(
            'npx',
            [
                'tsx',
                'scripts/seed-db.ts',
                '--url',
                PB_URL,
                '--admin-email',
                email,
                '--admin-pw',
                password,
            ],
            {
                stdio: 'inherit',
                env: process.env,
            }
        )

        seed.on('close', code => {
            if (code === 0) {
                resolve()
            } else {
                reject(new Error(`Seed script exited with code ${code}`))
            }
        })

        seed.on('error', reject)
    })
}

async function main() {
    let pb: ReturnType<typeof spawn> | null = null

    try {
        killExistingPocketBase()
        deletePbData()
        buildPocketBase()
        createSuperuser()
        pb = await startPocketBase()

        const ready = await waitForPocketBase()
        if (!ready) {
            throw new Error('PocketBase failed to start within timeout')
        }

        await runSeedScript()

        if (CONFIG.keepRunning) {
            await new Promise<void>(resolve => {
                process.on('SIGINT', () => {
                    resolve()
                })
                process.on('SIGTERM', () => {
                    resolve()
                })
            })
        }
    } catch (err) {
        console.error('[reset-dev-db] Failed:', err)
        process.exit(1)
    } finally {
        if (pb) {
            pb.kill('SIGTERM')
        }
    }
}

main()
