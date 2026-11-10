import { expect, test } from '@playwright/test'
import { signIn, trackConsoleErrors } from '../helpers'

test.describe('sign in', () => {
  test('a signed-out visitor is sent to the sign-in page', async ({ page }) => {
    const errors = trackConsoleErrors(page)

    await page.goto('/')
    await expect(page).toHaveURL(/\/sign-in/)
    // The card title is a div, not a heading: the page's one h1 is the app
    // name, so the button is what says the sign-in form rendered.
    await expect(page.getByText('Welcome back')).toBeVisible()
    await expect(page.getByRole('button', { name: /^Login with/ })).toBeVisible()

    // Nothing on this page needs an account, so a console error means the
    // bundle or the runtime config is broken.
    expect(errors).toEqual([])
  })

  test('the real sign-in flow reaches the dashboard', async ({ page }) => {
    // Console errors are not asserted here: this instance has no DNSPod
    // account, so the dashboard's domain list answers 400 and the browser logs
    // the failed fetch. That is the app reporting an empty install, not a
    // broken page; smoke.spec.ts covers console health on pages that render
    // without one.
    await signIn(page)
    await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible()
    // The subtitle greets by the local hour, so the assertion names all four
    // English greetings rather than whichever one the clock happens to pick.
    await expect(page.getByText(/^Good (morning|afternoon|evening|night), /)).toBeVisible()
  })

  // The token lives in sessionStorage and dies with the tab, which is what
  // makes a reload a new session rather than a remembered one.
  test('a reload keeps the session within the tab', async ({ page }) => {
    await signIn(page)
    await page.reload()
    await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible()
  })
})
