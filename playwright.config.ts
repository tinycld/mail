import { defineConfig, devices } from '@playwright/test'

process.loadEnvFile()

const TEST_PB_PORT = 7091
const TEST_EXPO_PORT = 7101

export default defineConfig({
    testDir: '.',
    testMatch: ['tests/e2e/**/*.spec.ts', 'packages/*/tests/**/*.spec.ts'],
    fullyParallel: true,
    forbidOnly: !!process.env.CI,
    retries: process.env.CI ? 2 : 0,
    workers: process.env.CI ? 1 : undefined,
    reporter: 'html',
    globalSetup: './tests/playwright-global-setup.ts',
    globalTeardown: './tests/playwright-global-teardown.ts',
    use: {
        baseURL: `http://localhost:${TEST_EXPO_PORT}`,
        trace: 'on-first-retry',
    },
    projects: [
        {
            name: 'chromium',
            use: { ...devices['Desktop Chrome'] },
        },
    ],
    webServer: {
        command: 'npm run dev:test:servers',
        url: `http://localhost:${TEST_EXPO_PORT}`,
        reuseExistingServer: !process.env.CI,
        timeout: 120_000,
        env: {
            PB_SERVER_ADDR: `http://127.0.0.1:${TEST_PB_PORT}`,
            VITE_PB_SERVER_ADDR: `http://127.0.0.1:${TEST_PB_PORT}`,
        },
    },
})
