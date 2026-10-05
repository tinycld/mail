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
// PAGE_SIZE (useThreadListItems.ts) plus one: the fewest threads that make a
// second page exist.
const THREAD_COUNT = 101
// Sequential delivery of 101 messages takes minutes; batches keep the spec
// inside the default test timeout.
const DELIVERY_BATCH = 20

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
        const address = await createSharedMailbox(page, 'Paging')
        const subjects = Array.from({ length: THREAD_COUNT }, (_, i) =>
            uniqueSubject(`Page${String(i).padStart(3, '0')}`)
        )
        // One second apart and fixed from the index: the Date header has
        // one-second resolution, so a burst would tie on latest_date, and
        // concurrent deliveries finish in any order.
        const firstDate = Date.now() - THREAD_COUNT * 1000
        for (let start = 0; start < THREAD_COUNT; start += DELIVERY_BATCH) {
            const batch = subjects.slice(start, start + DELIVERY_BATCH)
            await Promise.all(
                batch.map((subject, offset) =>
                    deliverInbound(request, {
                        subject,
                        to: address,
                        date: new Date(firstDate + (start + offset) * 1000),
                    })
                )
            )
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
