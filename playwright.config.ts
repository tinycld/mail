import path from 'node:path'
import { defineConfig } from '@playwright/test'
import appConfig from '@tinycld/core/playwright-config'

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
    // even after the sidebar testID wait.
    //
    // Tests that page.reload() (mail-thread delete + archive) re-pay the
    // chunk cost, so they need additional headroom — pushed to 90s after
    // observing the reload + navigateToPersonalInbox + openThread chain
    // still occasionally hitting 60s on the 2-core runner. Once the chunk
    // is cached for a worker, follow-up tests run fast.
    timeout: 90_000,
})
