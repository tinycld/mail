import { test, expect } from '@playwright/test'

const TEST_USER_EMAIL = process.env.TEST_USER_LOGIN || 'user@tinycld.org'
const TEST_USER_PASSWORD = process.env.TEST_USER_PW || 'TestUser1234!'
const ORG_SLUG = 'test-org'

async function login(page: import('@playwright/test').Page) {
    await page.goto('/')
    await page.getByPlaceholder('Email').fill(TEST_USER_EMAIL)
    await page.getByPlaceholder('Password').fill(TEST_USER_PASSWORD)
    await page.getByRole('button', { name: /sign in|log in/i }).click()
    await page.waitForURL(/\/app\//)
}

test.describe('Contacts', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
    })

    test('list screen renders with seed data', async ({ page }) => {
        await page.goto(`/app/${ORG_SLUG}/contacts`)
        await expect(page.getByText('Contacts (')).toBeVisible()
        await expect(page.getByText('Alice')).toBeVisible()
        await expect(page.getByText('Bob')).toBeVisible()
    })

    test('create a new contact and verify it appears', async ({ page }) => {
        await page.goto(`/app/${ORG_SLUG}/contacts/new`)
        await page.getByTestId('first_name').fill('Tester')
        await page.getByTestId('last_name').fill('McTest')
        await page.getByTestId('email').fill('tester@example.com')
        await page.getByTestId('phone').fill('555-000-1234')
        await page.getByRole('button', { name: /create/i }).click()

        await page.waitForURL(/\/contacts$/)
        await expect(page.getByText('Tester McTest')).toBeVisible()
    })

    test('click a contact, edit fields, save, verify changes persist', async ({ page }) => {
        await page.goto(`/app/${ORG_SLUG}/contacts`)
        await page.getByText('Alice').click()
        await page.waitForURL(/\/contacts\//)

        const firstNameInput = page.getByTestId('first_name')
        await firstNameInput.clear()
        await firstNameInput.fill('Alicia')

        await page.getByRole('button', { name: /save/i }).click()
        await page.goBack()

        await expect(page.getByText('Alicia')).toBeVisible()
    })

    test('toggle favorite from detail view', async ({ page }) => {
        await page.goto(`/app/${ORG_SLUG}/contacts`)
        await page.getByText('Bob').click()
        await page.waitForURL(/\/contacts\//)

        const starButton = page.locator('svg[data-testid="star"]').first()
            ?? page.locator('svg').filter({ has: page.locator('path') }).first()
        await starButton.click()
    })

    test('search filters contacts', async ({ page }) => {
        await page.goto(`/app/${ORG_SLUG}/contacts`)

        const searchInput = page.getByPlaceholder('Search contacts...')
        await searchInput.fill('alice')

        await expect(page.getByText('Alice')).toBeVisible()
        await expect(page.getByText('Bob')).not.toBeVisible()
    })
})
