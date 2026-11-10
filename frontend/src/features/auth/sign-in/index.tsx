import { useState } from 'react'
import { useSearch } from '@tanstack/react-router'
import { Loader2, Radio } from 'lucide-react'
import { signIn } from '@/lib/auth'
import { returnPathOf } from '@/lib/redirect'
import { useTranslation } from 'react-i18next'
import { authRequired, getConfig, providerName } from '@/lib/config'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
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
          <CardDescription>{t('signIn.description', { provider })}</CardDescription>
        </CardHeader>
        <CardContent>
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
        </CardContent>
      </Card>
      <p className='text-muted-foreground mt-6 max-w-sm px-4 text-center text-sm'>
        {t('signIn.blurb', { app: appName })}
      </p>
    </AuthLayout>
  )
}
