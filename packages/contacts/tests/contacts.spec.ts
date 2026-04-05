import { expect, test } from '@playwright/test'

const TEST_USER_EMAIL = process.env.TEST_USER_LOGIN || 'user@tinycld.org'
const TEST_USER_PASSWORD = process.env.TEST_USER_PW || 'TestUser1234!'
const ORG_SLUG = 'test-org'

async function login(page: import('@playwright/test').Page) {
    await page.goto('/')
    await page.getByPlaceholder('you@example.com').fill(TEST_USER_EMAIL)
    await page.getByPlaceholder('Password').fill(TEST_USER_PASSWORD)
    await page.getByText('Sign in', { exact: true }).last().click()
    await page.waitForURL(/\/app\//)
}

test.describe('Contacts', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
    })

    test('list screen renders with seed data', async ({ page }) => {
        await page.goto(`/a/${ORG_SLUG}/contacts`)
        await expect(page.getByText(/Contacts \(\d+\)/)).toBeVisible()
        await expect(page.getByRole('link', { name: /Alice Johnson/ })).toBeVisible()
        await expect(page.getByRole('link', { name: /Bob Smith/ })).toBeVisible()
    })

    test('create a new contact and verify it appears', async ({ page }) => {
        await page.goto(`/a/${ORG_SLUG}/contacts`)
        await page.getByText('+ Create contact').click()
        await page.waitForURL(/\/contacts\/new/)

        await page.getByTestId('first_name').fill('Tester')
        await page.getByTestId('last_name').fill('McTest')
        await page.getByTestId('email').fill('tester@example.com')
        await page.getByTestId('phone').fill('555-000-1234')
        await page.getByRole('button', { name: 'Create' }).click()

        await page.waitForURL(url => !url.pathname.includes('/new'), { timeout: 10_000 })
        await expect(page.getByRole('link', { name: /Tester McTest/ })).toBeVisible({
            timeout: 10_000,
        })
    })

    test('click a contact, edit fields, save, verify changes persist', async ({ page }) => {
        await page.goto(`/a/${ORG_SLUG}/contacts`)
        await page.getByRole('link', { name: /Alice Johnson/ }).click()
        await page.waitForURL(/\/contacts\//)

        const firstNameInput = page.getByTestId('first_name')
        await firstNameInput.clear()
        await firstNameInput.fill('Alicia')

        await page.getByRole('button', { name: /save/i }).click()
        await page.goBack()

        await expect(page.getByText('Alicia')).toBeVisible()
    })

    test('toggle favorite from detail view', async ({ page }) => {
        await page.goto(`/a/${ORG_SLUG}/contacts`)
        await page.getByRole('link', { name: /Bob Smith/ }).click()
        await page.waitForURL(/\/contacts\//)

        // Click the star/favorite toggle button
        const favoriteButton = page
            .locator('[data-testid="favorite-toggle"]')
            .or(page.locator('svg').filter({ hasText: '' }).first())
        await favoriteButton.click({ timeout: 10_000 })
    })

    test('search filters contacts', async ({ page }) => {
        await page.goto(`/a/${ORG_SLUG}/contacts`)

        const searchInput = page.getByPlaceholder('Search contacts...')
        await searchInput.fill('carol')

        await expect(page.getByRole('link', { name: /Carol Williams/ })).toBeVisible()
        await expect(page.getByRole('link', { name: /Bob Smith/ })).not.toBeVisible()
    })
})
