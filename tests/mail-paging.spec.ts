import { expect, test } from '@playwright/test'
import { login, navigateToPackage } from '@tinycld/core/e2e-helpers'
import {
    createSharedMailbox,
    deliverInbound,
    emailRow,
    openSharedMailbox,
    uniqueSubject,
} from './helpers'

// Pages are cursor pages: the boundary is part of the query, so page two
// loads only its rows. The total comes from the folder counts table.
test.describe('Mail — Paging', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'mail', {
            waitFor: page.getByTestId('package-sidebar-mounted'),
        })
    })

    test('next and prev walk 101 threads in two pages with a correct total', async ({
        page,
        request,
    }) => {
        // A data-volume budget for 101 webhook deliveries, not a flake workaround.
        test.setTimeout(240_000)
        const address = await createSharedMailbox(page, 'Paging')
        const subjects: string[] = []
        // One second apart: the Date header has one-second resolution, and a
        // burst of deliveries in the same second would tie on latest_date.
        const firstDate = Date.now() - 101 * 1000
        for (let i = 0; i < 101; i++) {
            const subject = uniqueSubject(`Page${String(i).padStart(3, '0')}`)
            subjects.push(subject)
            const date = new Date(firstDate + i * 1000)
            await deliverInbound(request, { subject, to: address, date })
        }
        // The oldest message sorts last, so it alone is on page two.
        const oldest = subjects[0]
        const newest = subjects[subjects.length - 1]

        await openSharedMailbox(page, address)
        await expect(emailRow(page, newest)).toBeVisible()
        await expect(page.getByText('1–100 of 101')).toBeVisible()
        await expect(emailRow(page, oldest)).toHaveCount(0)

        // getByRole, not getByLabel: a hidden second copy of the toolbar
        // carries the same aria-label, and getByRole skips hidden elements.
        await page.getByRole('button', { name: 'Older', exact: true }).click()
        await expect(page).toHaveURL(/cursor=/)
        await expect(emailRow(page, oldest)).toBeVisible()
        await expect(page.getByText('101–101 of 101')).toBeVisible()
        await expect(emailRow(page, newest)).toHaveCount(0)

        await page.getByRole('button', { name: 'Newer', exact: true }).click()
        await expect(page).not.toHaveURL(/cursor=/)
        await expect(emailRow(page, newest)).toBeVisible()
    })
})
