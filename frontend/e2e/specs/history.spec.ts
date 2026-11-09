import { expect, test } from '@playwright/test'
import { signIn } from '../helpers'

/**
 * The two timelines share one page, and the import feeds the second. The CSV
 * goes through the same hidden input the button clicks, so the suite covers the
 * browser path rather than calling the API around it.
 */
const CSV =
  [
    'time,ip,family',
    '2026-01-01T00:00:00Z,203.0.113.10,v4',
    '2026-01-02T00:00:00Z,203.0.113.24,v4',
    '2026-01-03T00:00:00Z,2001:db8::10,v6',
    // The same address a second time is not a change, so the server skips it.
    '2026-01-04T00:00:00Z,203.0.113.24,v4',
  ].join('\n') + '\n'

test.describe('history', () => {
  test.beforeEach(async ({ page }) => {
    await signIn(page)
    await page.goto('/history')
    await expect(page.getByRole('heading', { name: 'History' })).toBeVisible()
  })

  test('task history and IP history are tabs on one page', async ({ page }) => {
    await expect(page.getByRole('tab', { name: 'Task History' })).toHaveAttribute(
      'data-state',
      'active'
    )

    await page.getByRole('tab', { name: 'IP History' }).click()
    await expect(page.getByRole('tab', { name: 'IP History' })).toHaveAttribute(
      'data-state',
      'active'
    )
    // Nothing has run on this instance, so the timeline is empty rather than
    // asking for a missing account.
    await expect(page.getByText('No address change has been recorded yet.').first()).toBeVisible()
  })

  test('a CSV import lands in the timeline', async ({ page }) => {
    await page.getByRole('tab', { name: 'IP History' }).click()
    await expect(page.getByText('No address change has been recorded yet.').first()).toBeVisible()

    await page.locator('input[type=file]').setInputFiles({
      name: 'history.csv',
      mimeType: 'text/csv',
      buffer: Buffer.from(CSV),
    })

    // Three rows stored, the repeated address skipped, and the row it was on
    // named in the toast rather than silently dropped.
    await expect(page.getByText('Imported 3, skipped 1.')).toBeVisible()
    // Only the line number is asserted: the wording belongs to the locale and
    // a copy edit must not fail the suite.
    await expect(page.getByText(/Line 5/)).toBeVisible()
    await expect(page.getByText('3 address changes')).toBeVisible()
    await expect(page.locator('tbody tr')).toHaveCount(3)

    // The first address of a family is a baseline: it is listed with a dash on
    // the left, and the chart has no bar for it.
    await expect(page.getByRole('cell', { name: /203\.0\.113\.10/ }).first()).toBeVisible()
  })
})
