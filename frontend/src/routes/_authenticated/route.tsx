import { createFileRoute, redirect } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { authRequired } from '@/lib/config'
import { AuthenticatedLayout } from '@/components/layout/authenticated-layout'

export const Route = createFileRoute('/_authenticated')({
  beforeLoad: ({ location }) => {
    // The session was resolved before the router mounted, so this is a plain
    // check; renewals happen on demand when a request needs a token.
    //
    // With `authRequired` off the server takes every request without a token,
    // so asking for one here would only lock the pages behind nothing.
    if (authRequired() && !useAuthStore.getState().auth.user) {
      throw redirect({ to: '/sign-in', search: { redirect: location.href } })
    }
  },
  component: AuthenticatedLayout,
})
