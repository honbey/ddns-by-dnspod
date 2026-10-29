#!/usr/bin/env node
/**
 * Writes the SPA's runtime configuration next to the built bundle.
 *
 * The committed public/config.json holds placeholders, so a deployment that
 * never sets any variable would still point at auth.example.com. Rather than
 * introducing a second set of VITE_* variables, this reuses the values already
 * used for the deploy: one place to configure, two consumers.
 *
 * Runs as `postbuild`, after Vite has copied public/config.json into the output
 * directory. Local development keeps reading public/config.json directly.
 */

import { access } from 'node:fs/promises';
import { mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { resolve, dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

async function exists(path) {
  return access(path).then(
    () => true,
    () => false,
  );
}

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const publicDir = join(root, 'public');
const localPath = join(publicDir, 'config.local.json');
const defaultsPath = join(publicDir, 'config.json');
// A checkout can carry its own defaults: config.local.json is merged over the
// tracked file, so a local build can override one setting - authRequired, say -
// without having to repeat the whole of it.
const tracked = JSON.parse(await readFile(defaultsPath, 'utf8'));
const local = (await exists(localPath))
  ? JSON.parse(await readFile(localPath, 'utf8'))
  : {};
// Vite writes into the Go embed directory, so that is where the runtime
// configuration has to land: it ships inside the binary next to the bundle.
const outDir = resolve(
  root,
  process.env.BUILD_OUT_DIR || '../backend/internal/frontend/dist',
);

const defaults = {
  ...tracked,
  ...local,
  oidc: { ...tracked.oidc, ...(local.oidc ?? {}) },
};

const env = process.env;

const config = {
  appName: env.APP_NAME || defaults.appName,
  apiBaseUrl: env.API_BASE_URL || defaults.apiBaseUrl,
  // Mirrors the server's AUTH_DISABLED: a development deployment takes every
  // request without a token, and asking the browser for one would only lock the
  // pages behind nothing.
  authRequired:
    env.AUTH_REQUIRED === undefined
      ? defaults.authRequired
      : env.AUTH_REQUIRED !== 'false',
  // Mirrors the server's acme.enabled, for the deployments where something
  // other than this server answers /config.json. The server itself reports the
  // flag it is running with.
  acmeEnabled:
    env.ACME_ENABLED === undefined
      ? (defaults.acmeEnabled ?? true)
      : env.ACME_ENABLED !== 'false',
  oidc: {
    issuer: (env.OIDC_ISSUER || defaults.oidc.issuer).replace(/\/+$/, ''),
    clientId: env.OIDC_CLIENT_ID || defaults.oidc.clientId,
    scope: env.OIDC_SCOPE || defaults.oidc.scope,
    redirectPath: env.OIDC_REDIRECT_PATH || defaults.oidc.redirectPath,
    providerName: env.OIDC_PROVIDER_NAME || defaults.oidc.providerName || '',
    logoutUrlTemplate: (
      env.OIDC_LOGOUT_URL ||
      defaults.oidc.logoutUrlTemplate ||
      ''
    ).replace(/\/+$/, ''),
  },
};

await mkdir(outDir, { recursive: true });
await writeFile(join(outDir, 'config.json'), `${JSON.stringify(config, null, 2)}\n`);
// A built bundle keeps only the merged config.json. Leaving the local file next
// to it would let the SPA pick the unmerged one - it probes config.local.json
// first - and on a server that answers /config.json at run time that would
// shadow the deployment's own settings. The dev server serves public/ directly,
// so it is unaffected.
await rm(join(outDir, 'config.local.json'), { force: true });

console.log(
  `  config.json -> issuer=${config.oidc.issuer} clientId=${config.oidc.clientId} api=${config.apiBaseUrl}` +
    ` provider=${config.oidc.providerName || 'OIDC'}` +
    ` logout=${config.oidc.logoutUrlTemplate || '(end_session)'}` +
    ` authRequired=${config.authRequired}`,
);
