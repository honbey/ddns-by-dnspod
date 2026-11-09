import { useTranslation } from 'react-i18next'
import { ConfigDrawer } from '@/components/config-drawer'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { PageHeading } from '@/components/layout/page-heading'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { Search } from '@/components/search'
import { ThemeSwitch } from '@/components/theme-switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { IPHistory } from './components/ip-history'
import { TaskHistory } from './components/task-history'

/**
 * Two timelines under one heading.
 *
 * They answer different questions - what the tasks wrote to DNS, and what this
 * host's address was when they wrote it - but they are read the same way and
 * a sidebar entry each would say they are separate concerns. Tabs keep the
 * address history next to the task history without making either the parent.
 */
export function History() {
  const { t } = useTranslation()

  return (
    <>
      <Header>
        <Search className='me-auto' />
        <ThemeSwitch />
        <ConfigDrawer />
        <ProfileDropdown />
      </Header>

      <Main className='flex flex-1 flex-col gap-4'>
        <PageHeading title={t('nav.history')} subtitle={t('history.pageSubtitle')} />

        <Tabs defaultValue='tasks' className='flex flex-1 flex-col gap-4'>
          <TabsList aria-label={t('history.tabsAria')}>
            <TabsTrigger value='tasks'>{t('history.tabTasks')}</TabsTrigger>
            <TabsTrigger value='ip'>{t('history.tabIP')}</TabsTrigger>
          </TabsList>

          <TabsContent value='tasks' className='flex flex-1 flex-col'>
            <TaskHistory />
          </TabsContent>
          <TabsContent value='ip' className='flex flex-1 flex-col'>
            <IPHistory />
          </TabsContent>
        </Tabs>
      </Main>
    </>
  )
}
