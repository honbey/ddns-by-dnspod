import { expect, test } from '@playwright/test'

test.describe('probes', () => {
  // A container's healthcheck is /api/ready, so both have to answer on a
  // deployment that looks healthy.
  test('liveness and readiness answer', async ({ request }) => {
    expect((await request.get('/api/health')).status()).toBe(200)
    expect((await request.get('/api/ready')).status()).toBe(200)
  })

  // The SPA is told at run time which provider to sign in with; a wrong value
  // here sends the browser to an IdP that has never heard of the client.
  test('the runtime config describes this deployment', async ({ request }) => {
    const response = await request.get('/config.json')
    expect(response.status()).toBe(200)

    const body = await response.json()
    expect(body.authRequired).toBe(true)
    expect(body.oidc.providerName).toBe('Local IdP')
    expect(body.oidc.issuer).toContain('127.0.0.1')
  })

  // An API path that does not exist must not fall through to the SPA: a client
  // parsing HTML as JSON reports the wrong problem.
  test('an unknown API path answers JSON', async ({ request }) => {
    const response = await request.get('/api/does-not-exist')
    expect(response.status()).toBe(404)
    expect(response.headers()['content-type']).toContain('application/json')
  })
})
