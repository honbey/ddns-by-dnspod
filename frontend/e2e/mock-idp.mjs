#!/usr/bin/env node
/**
 * A tiny OpenID Connect provider for local development, with no dependencies.
 *
 * It speaks exactly the flows this app uses - authorization code with PKCE for
 * the browser, a signed JWT access token for the resource server - and nothing
 * more: no registration, no refresh tokens, no consent screen. A login form
 * that accepts any username stands in for the real authentication step.
 *
 *   node e2e/mock-idp.mjs            # listens on 127.0.0.1:9443
 *   PORT=9444 node e2e/mock-idp.mjs  # or wherever
 *
 * Point the frontend's config.local.json and the backend's OIDC_ISSUER at
 * http://127.0.0.1:9443 and both ends talk to this process.
 */

import {
  createHash,
  createSign,
  generateKeyPairSync,
  randomBytes,
  verify as verifySignature,
} from 'node:crypto'
import { createServer } from 'node:http'

const PORT = Number(process.env.PORT || 9443)
const ISSUER = process.env.ISSUER || `http://127.0.0.1:${PORT}`
const CLIENT_ID = process.env.CLIENT_ID || 'local-client'
const ACCESS_TOKEN_TTL = Number(process.env.TOKEN_TTL || 3600)
const CODE_TTL = 120

/** One signing key for the whole process lifetime; restarts rotate it. */
const { publicKey, privateKey } = generateKeyPairSync('rsa', {
  modulusLength: 2048,
})
const KID = 'mock-key-1'

/** authorization codes, one-time and short-lived: code -> grant. */
const codes = new Map()

/** refresh tokens, one per issued grant: token -> { sub, clientId, scope }. */
const refreshTokens = new Map()

/**
 * How long a just-rotated refresh token still works.
 *
 * Rotation makes every refresh token one-use, but two callers can be in flight
 * at once - two tabs, or a page load interrupting an in-flight renewal - and
 * the slower one presents the token that was just rotated away. Real providers
 * accept it briefly for exactly this reason; without the window the loser
 * loses its session over a race.
 */
const REFRESH_REUSE_GRACE_MS = 30_000

const b64url = (input) => Buffer.from(input).toString('base64url')
const sha256b64url = (input) =>
  createHash('sha256').update(input).digest('base64url')

function jwkOf(publicKey) {
  const jwk = publicKey.export({ format: 'jwk' })
  return {
    kty: 'RSA',
    use: 'sig',
    alg: 'RS256',
    kid: KID,
    n: jwk.n,
    e: jwk.e,
  }
}

function signJWT(claims) {
  const header = b64url(JSON.stringify({ alg: 'RS256', typ: 'JWT', kid: KID }))
  const body = b64url(JSON.stringify(claims))
  const signature = createSign('RSA-SHA256')
    .update(`${header}.${body}`)
    .sign(privateKey, 'base64url')
  return `${header}.${body}.${signature}`
}

function parseJWT(token) {
  const [header, body, signature] = token.split('.')
  if (!header || !body || !signature) return null
  // crypto.verify(algorithm, data, key, signature): the key comes first.
  const ok = verifySignature(
    'RSA-SHA256',
    `${header}.${body}`,
    publicKey,
    Buffer.from(signature, 'base64url')
  )
  if (!ok) return null
  try {
    return JSON.parse(Buffer.from(body, 'base64url').toString())
  } catch {
    return null
  }
}

function cors(res) {
  res.setHeader('Access-Control-Allow-Origin', '*')
  res.setHeader('Access-Control-Allow-Methods', 'GET, POST, OPTIONS')
  res.setHeader(
    'Access-Control-Allow-Headers',
    'Authorization, Content-Type'
  )
}

function sendJSON(res, status, body) {
  cors(res)
  res.writeHead(status, { 'Content-Type': 'application/json' })
  res.end(JSON.stringify(body))
}

function sendHTML(res, status, html) {
  res.writeHead(status, { 'Content-Type': 'text/html; charset=utf-8' })
  res.end(html)
}

function redirect(res, location) {
  res.writeHead(302, { Location: location })
  res.end()
}

/** Everything interpolated into the form below comes from the query string. */
const escapeHtml = (value) =>
  String(value).replace(
    /[&<>"']/g,
    (ch) =>
      ({
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#39;',
      })[ch]
  )

function page(title, body) {
  return `<!doctype html>
<html>
<head><meta charset="utf-8"><title>${escapeHtml(title)}</title></head>
<body style="font-family: system-ui; max-width: 24rem; margin: 4rem auto">
${body}
</body>
</html>`
}

/** The login form: any username is accepted, that is what "mock" means here. */
function loginForm(query) {
  const hidden = Object.entries(query)
    .filter(([key]) => key !== 'login')
    .map(
      ([key, value]) =>
        `<input type="hidden" name="${escapeHtml(key)}" value="${escapeHtml(value)}">`
    )
    .join('\n')
  return page(
    'Sign in - mock IdP',
    `
<h2>Mock identity provider</h2>
<p style="color:#666">Any username works. This is not a real login.</p>
<form method="POST" action="/authorize/login">
${hidden}
<label>Username<br><input name="login" value="admin" autofocus></label>
<p><button type="submit">Sign in</button></p>
</form>`
  )
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    let data = ''
    req.on('data', (chunk) => {
      data += chunk
      if (data.length > 1e6) reject(new Error('body too large'))
    })
    req.on('end', () => resolve(new URLSearchParams(data)))
    req.on('error', reject)
  })
}

const server = createServer(async (req, res) => {
  // One bad request must not take the provider down with it.
  try {
    await handle(req, res)
  } catch (error) {
    console.error('[mock-idp] request failed:', error)
    if (!res.headersSent) sendJSON(res, 500, { error: 'server_error' })
  }
})

async function handle(req, res) {
  const url = new URL(req.url, ISSUER)
  const path = url.pathname
  const q = Object.fromEntries(url.searchParams)

  if (req.method === 'OPTIONS') {
    cors(res)
    res.writeHead(204)
    return res.end()
  }

  /* ---- discovery ---- */
  if (path === '/.well-known/openid-configuration') {
    return sendJSON(res, 200, {
      issuer: ISSUER,
      authorization_endpoint: `${ISSUER}/authorize`,
      token_endpoint: `${ISSUER}/token`,
      userinfo_endpoint: `${ISSUER}/userinfo`,
      jwks_uri: `${ISSUER}/jwks.json`,
      response_types_supported: ['code'],
      grant_types_supported: ['authorization_code', 'refresh_token'],
      subject_types_supported: ['public'],
      id_token_signing_alg_values_supported: ['RS256'],
      token_endpoint_auth_methods_supported: ['none'],
      code_challenge_methods_supported: ['S256'],
      scopes_supported: ['openid', 'profile', 'email', 'groups'],
      claims_supported: [
        'sub',
        'preferred_username',
        'name',
        'email',
        'groups',
      ],
    })
  }

  if (path === '/jwks.json') {
    return sendJSON(res, 200, { keys: [jwkOf(publicKey)] })
  }

  /* ---- authorization endpoint ---- */
  if (path === '/authorize' && req.method === 'GET') {
    if (q.response_type !== 'code') {
      return sendHTML(res, 400, page('Error', 'response_type must be "code"'))
    }
    if (q.client_id !== CLIENT_ID) {
      return sendHTML(
        res,
        400,
        page('Error', `unknown client ${escapeHtml(q.client_id ?? '(none)')}`)
      )
    }
    if (q.code_challenge_method !== 'S256' || !q.code_challenge) {
      return sendHTML(res, 400, page('Error', 'PKCE (S256) is required'))
    }
    if (!/^https?:\/\//.test(q.redirect_uri ?? '')) {
      return sendHTML(res, 400, page('Error', 'a redirect_uri is required'))
    }
    // A username in the query is the "already signed in" shortcut, used by the
    // smoke test; humans get the form.
    if (q.login) {
      return issueCode(res, q, q.login)
    }
    return sendHTML(res, 200, loginForm(q))
  }

  if (path === '/authorize/login' && req.method === 'POST') {
    const form = await readBody(req)
    const q = Object.fromEntries(form)
    const username = (q.login || 'admin').trim() || 'admin'
    return issueCode(res, q, username)
  }

  /* ---- token endpoint ---- */
  if (path === '/token' && req.method === 'POST') {
    const form = await readBody(req)
    const grantType = form.get('grant_type')

    if (grantType === 'refresh_token') {
      const given = form.get('refresh_token') ?? ''
      const session = refreshTokens.get(given)
      if (!session) {
        return sendJSON(res, 400, {
          error: 'invalid_grant',
          error_description: 'unknown refresh token',
        })
      }
      if (session.rotatedAt && Date.now() - session.rotatedAt > REFRESH_REUSE_GRACE_MS) {
        refreshTokens.delete(given)
        return sendJSON(res, 400, {
          error: 'invalid_grant',
          error_description: 'refresh token already used',
        })
      }
      // Mark it rotated rather than deleting it: the reuse grace window above
      // is what a concurrent renewal slips through.
      session.rotatedAt = session.rotatedAt ?? Date.now()
      return issueTokens(res, session, true)
    }

    if (grantType !== 'authorization_code') {
      return sendJSON(res, 400, {
        error: 'unsupported_grant_type',
        error_description: 'only authorization_code and refresh_token are supported',
      })
    }

    const code = form.get('code') ?? ''
    const grant = codes.get(code)
    if (!grant) {
      return sendJSON(res, 400, {
        error: 'invalid_grant',
        error_description: 'unknown, expired or already used code',
      })
    }
    if (form.get('client_id') !== grant.clientId) {
      return sendJSON(res, 400, { error: 'invalid_grant', error_description: 'client mismatch' })
    }
    if (form.get('redirect_uri') !== grant.redirectURI) {
      return sendJSON(res, 400, { error: 'invalid_grant', error_description: 'redirect_uri mismatch' })
    }
    const verifier = form.get('code_verifier') ?? ''
    if (sha256b64url(verifier) !== grant.challenge) {
      return sendJSON(res, 400, {
        error: 'invalid_grant',
        error_description: 'PKCE verification failed',
      })
    }
    // One-time, and only on success: a failed attempt leaves the code alone,
    // so a browser that retried with the right verifier is not locked out.
    codes.delete(code)

    return issueTokens(res, grant, /\boffline_access\b/.test(grant.scope ?? ''))
  }

  /* ---- userinfo ---- */
  if (path === '/userinfo' && req.method === 'GET') {
    const token = (req.headers.authorization ?? '').replace(/^Bearer\s+/i, '')
    const claims = token ? parseJWT(token) : null
    if (!claims || claims.iss !== ISSUER || claims.exp * 1000 < Date.now()) {
      res.setHeader('WWW-Authenticate', 'Bearer')
      return sendJSON(res, 401, { error: 'invalid_token' })
    }
    return sendJSON(res, 200, {
      sub: claims.sub,
      preferred_username: claims.preferred_username,
      name: claims.name,
      email: claims.email,
      groups: claims.groups,
    })
  }

  sendJSON(res, 404, { error: 'not_found' })
}

/**
 * Issues the token pair for a grant. A grant that asked for offline_access -
 * or came in on a refresh token - also gets a rotating refresh token, which is
 * what lets the app renew a session without an iframe.
 */
function issueTokens(res, session, withRefresh) {
  const now = Math.floor(Date.now() / 1000)
  const base = {
    iss: ISSUER,
    sub: session.sub,
    aud: session.clientId,
    iat: now,
    exp: now + ACCESS_TOKEN_TTL,
    preferred_username: session.sub,
    name: session.sub,
    email: `${session.sub}@example.com`,
    groups: ['app-admins'],
    scope: session.scope ?? 'openid',
  }
  console.log(
    `[token] issued for ${session.sub} (aud ${session.clientId}, ttl ${ACCESS_TOKEN_TTL}s${withRefresh ? ', with refresh' : ''})`
  )

  const body = {
    access_token: signJWT(base),
    id_token: signJWT(base),
    token_type: 'Bearer',
    expires_in: ACCESS_TOKEN_TTL,
    scope: session.scope ?? 'openid',
  }
  if (withRefresh) {
    const refresh = randomBytes(24).toString('base64url')
    refreshTokens.set(refresh, {
      sub: session.sub,
      clientId: session.clientId,
      scope: session.scope,
    })
    body.refresh_token = refresh
  }
  return sendJSON(res, 200, body)
}

/**
 * Creates the one-time code and sends the browser back to the client.
 *
 * The challenge travels with the code so /token can check the verifier later;
 * it is public information, the verifier never reaches this server.
 */
function issueCode(res, q, username) {
  const code = randomBytes(24).toString('base64url')
  codes.set(code, {
    challenge: q.code_challenge,
    redirectURI: q.redirect_uri,
    clientId: q.client_id,
    sub: username,
    scope: q.scope ?? 'openid',
    expires: Date.now() + CODE_TTL * 1000,
  })
  // Anything older than its TTL goes now; the map stays small either way.
  for (const [key, grant] of codes) {
    if (grant.expires < Date.now()) codes.delete(key)
  }

  const target = new URL(q.redirect_uri)
  target.searchParams.set('code', code)
  if (q.state) target.searchParams.set('state', q.state)
  console.log(`[authorize] code issued for ${username} -> ${target.origin}${target.pathname}`)
  redirect(res, target.toString())
}

// Sweep expired codes and rotated refresh tokens so a stale one cannot be
// replayed after its window.
setInterval(() => {
  for (const [key, grant] of codes) {
    if (grant.expires < Date.now()) codes.delete(key)
  }
  for (const [key, session] of refreshTokens) {
    if (session.rotatedAt && Date.now() - session.rotatedAt > REFRESH_REUSE_GRACE_MS) {
      refreshTokens.delete(key)
    }
  }
}, 30_000).unref()

server.listen(PORT, '127.0.0.1', () => {
  console.log(`[mock-idp] listening on ${ISSUER}`)
  console.log(`[mock-idp] discovery ${ISSUER}/.well-known/openid-configuration`)
  console.log(`[mock-idp] registered client_id: ${CLIENT_ID}`)
  console.log(`[mock-idp] any username signs in; token ttl ${ACCESS_TOKEN_TTL}s`)
})
