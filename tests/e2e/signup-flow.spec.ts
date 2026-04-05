import { expect, test } from '@playwright/test'

const UNIQUE = Date.now()
const SIGNUP_EMAIL = `signup-${UNIQUE}@test.tinycld.com`
const SIGNUP_PASSWORD = 'TestPass1234!'
const ORG_NAME = `Test Org ${UNIQUE}`

test.describe('Signup flow', () => {
    test('sign up, log out, then sign in with the new account', async ({ page }) => {
        await page.goto('/')

        // Switch to the signup form
        await page.getByText("Don't have an account?").click()
        await expect(page.getByText('Create account').first()).toBeVisible()

        // Fill out signup form
        await page.getByPlaceholder('Acme Corp').fill(ORG_NAME)
        await page.getByPlaceholder('you@example.com').fill(SIGNUP_EMAIL)
        await page.getByPlaceholder('At least 8 characters').fill(SIGNUP_PASSWORD)

        // Submit signup
        await page.getByText('Create account').last().click()

        // Should redirect to the app after signup + auto-login
        await page.waitForURL(/\/a\//, { timeout: 15_000 })

        // Sign out
        await page.getByLabel('Sign out').click()

        // Should be back at the login screen
        await expect(page.getByText('Sign in').first()).toBeVisible({ timeout: 10_000 })

        // Sign in with the account we just created
        await page.getByPlaceholder('you@example.com').fill(SIGNUP_EMAIL)
        await page.getByPlaceholder('Password').fill(SIGNUP_PASSWORD)
        await page.getByText('Sign in', { exact: true }).last().click()

        // Should redirect to the app
        await page.waitForURL(/\/a\//, { timeout: 10_000 })
    })
})
