import path from 'node:path'
import { defineConfig } from '@playwright/test'
import appConfig from '../app/playwright.config'

const WS_ROOT = path.resolve(import.meta.dirname, '..')
const TEST_DIR = path.join(WS_ROOT, 'node_modules', '@tinycld', 'mail', 'tests')

export default defineConfig({
    ...appConfig,
    testDir: TEST_DIR,
    // Per-test timeout. Default is 30s; mail's tests need more headroom
    // because each spec's first navigation pays a cold Metro compile for
    // the lazy mail-screen chunk (the entry bundle is pre-warmed by
    // globalSetup but per-package lazy chunks are not). Under CI load on
    // a 2-core ubuntu runner the first-mount-per-worker can take ~30-45s
    // even after the sidebar testID wait — pushing the per-test budget to
    // 60s keeps the beforeEach hook from timing out before the chunk
    // arrives. Once the chunk is cached, follow-up tests run fast.
    timeout: 60_000,
})
