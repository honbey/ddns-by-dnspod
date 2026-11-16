import { useState } from 'react'
import { useSearch } from '@tanstack/react-router'
import { Loader2, Radio } from 'lucide-react'
import { signIn } from '@/lib/auth'
import { returnPathOf } from '@/lib/redirect'
import { useTranslation } from 'react-i18next'
import { authRequired, getConfig, providerName } from '@/lib/config'
import signInIllustration from '@/assets/sign-in-illustration.svg'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { AuthLayout } from '../auth-layout'

/**
 * One way in: the browser signs in on its own and carries the answer.
 *
 * The server never sees this page - it only ever checks the token that comes
 * back - so there is nothing here for a deployment that asks for no token, and
 * such a deployment sends a visitor on rather than asking them to log in.
 */
export function SignIn() {
  const { t } = useTranslation()
  const { redirect } = useSearch({ from: '/(auth)/sign-in' })
  const [pending, setPending] = useState(false)
  const provider = providerName()
  const { appName } = getConfig()

  const handleSignIn = () => {
    setPending(true)
    // Full-page redirect; the button stays disabled so a slow provider cannot
    // be triggered twice from a double click.
    void signIn(returnPathOf(redirect ?? '/')).catch(() => setPending(false))
  }

  return (
    <AuthLayout>
      <Card className='w-full max-w-sm gap-6'>
        <CardHeader className='items-center text-center'>
          <CardTitle className='text-2xl font-bold tracking-tight'>{t('signIn.welcome')}</CardTitle>
        </CardHeader>
        <CardContent className='space-y-4'>
          <img src={signInIllustration} alt='' aria-hidden='true' className='mx-auto h-48 w-auto' />
          {authRequired() ? (
            <Button variant='outline' className='w-full' disabled={pending} onClick={handleSignIn}>
              {pending && <Loader2 className='animate-spin' />}
              {t('signIn.loginWith', { provider })}
            </Button>
          ) : (
            <Button variant='outline' className='w-full' asChild>
              <a href='/'>
                <Radio />
                {t('signIn.openDashboard')}
              </a>
            </Button>
          )}
          <p className='text-muted-foreground text-center text-sm'>
            {t('signIn.blurb', { app: appName })}
          </p>
        </CardContent>
      </Card>
    </AuthLayout>
  )
}
