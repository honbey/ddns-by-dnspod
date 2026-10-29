import { expect, type Page } from '@playwright/test'

/**
 * Completes the provider's login form. Assumes the app has already redirected
 * to /sign-in and the sign-in button has been pressed.
 */
export async function completeSignIn(page: Page, username = 'admin'): Promise<void> {
  await expect(page.getByRole('heading', { name: 'Mock identity provider' })).toBeVisible()
  await page.getByLabel('Username').fill(username)
  await page.getByRole('button', { name: 'Sign in' }).click()
}

/**
 * Signs in through the local identity provider and asserts where the app lands:
 * `landing` is the path the guard returns to, or '/' for a fresh visit.
 */
export async function signIn(
  page: Page,
  username = 'ops@example.com',
  landing = '/'
): Promise<void> {
  await page.goto('/')
  const loginButton = page.getByRole('button', { name: /^Login with/ })
  await expect(loginButton).toBeVisible()
  await loginButton.click()
  await completeSignIn(page, username)
  await expect(page).toHaveURL(landing)
}

/** Collects console errors and uncaught exceptions for the whole test. */
export function trackConsoleErrors(page: Page): string[] {
  const errors: string[] = []
  page.on('console', (message) => {
    if (message.type() === 'error') errors.push(message.text())
  })
  page.on('pageerror', (error) => errors.push(String(error)))
  return errors
}
