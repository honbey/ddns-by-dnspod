/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** OIDC issuer (discovery URL) the browser signs in against */
  readonly VITE_OIDC_ISSUER?: string
  /** Client id registered with that issuer, a public client using PKCE */
  readonly VITE_OIDC_CLIENT_ID?: string
  /** Scopes to ask for, defaults to "openid profile email groups" */
  readonly VITE_OIDC_SCOPE?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
