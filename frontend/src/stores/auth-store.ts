import { type User } from 'oidc-client-ts'
import { create } from 'zustand'

interface AuthState {
  auth: {
    user: User | null
    setUser: (user: User | null) => void
    /**
     * Whether the provider can end its own session, i.e. whether the
     * "sign out everywhere" entry applies. Resolved once per page load because
     * it may need the discovery document.
     */
    providerLogout: boolean
    setProviderLogout: (available: boolean) => void
    reset: () => void
  }
}

/**
 * Holds the OIDC session for the router and for anything that needs to react
 * to it. The token itself stays inside oidc-client-ts; consumers ask
 * `getAccessToken()` for it so expired tokens are renewed in one place.
 */
export const useAuthStore = create<AuthState>()((set) => ({
  auth: {
    user: null,
    setUser: (user) => set((state) => ({ ...state, auth: { ...state.auth, user } })),
    providerLogout: false,
    setProviderLogout: (available) =>
      set((state) => ({
        ...state,
        auth: { ...state.auth, providerLogout: available },
      })),
    reset: () => set((state) => ({ ...state, auth: { ...state.auth, user: null } })),
  },
}))
