import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { CheckCircle2, TriangleAlert } from 'lucide-react'
import { type DDNSTask } from '@/lib/types'
import { formatTime } from '@/lib/format'
import { hostOf } from '@/features/tasks/components/tasks-columns'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

type TaskHealthProps = {
  tasks: DDNSTask[]
  failing: DDNSTask[]
  loading: boolean
}

/**
 * Whether the schedule is actually working, with the failures named.
 *
 * A count of failing tasks is on the metrics row already; what belongs here is
 * which ones and why, because that is the difference between noticing and
 * being able to fix.
 */
export function TaskHealth({ tasks, failing, loading }: TaskHealthProps) {
  const { t } = useTranslation()
  const healthy = tasks.length - failing.length

  return (
    <Card className='h-full'>
      <CardHeader>
        <CardTitle>{t('dashboard.taskHealth')}</CardTitle>
        <CardDescription>
          {loading
            ? t('dashboard.checking')
            : tasks.length === 0
              ? t('dashboard.nothingScheduled')
              : t('dashboard.healthyOf', { healthy, total: tasks.length })}
        </CardDescription>
        <CardAction>
          <Button asChild variant='ghost' size='sm'>
            <Link to='/tasks'>{t('dashboard.manage')}</Link>
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent>
        {loading ? (
          <div className='space-y-4'>
            {Array.from({ length: 3 }).map((_, index) => (
              <Skeleton key={index} className='h-9 w-full' />
            ))}
          </div>
        ) : tasks.length === 0 ? (
          <p className='text-muted-foreground py-4 text-center text-sm'>
            {t('dashboard.addTaskHint')}
          </p>
        ) : failing.length === 0 ? (
          <div className='flex items-center gap-2 py-2 text-sm'>
            <CheckCircle2 aria-hidden='true' className='size-4 shrink-0' />
            {t('dashboard.everyTaskSucceeded')}
          </div>
        ) : (
          <ul className='space-y-4'>
            {failing.map((task) => (
              <li key={task.id} className='flex items-start gap-3'>
                <TriangleAlert
                  aria-hidden='true'
                  className='mt-0.5 size-4 shrink-0 text-destructive'
                />
                <div className='min-w-0 flex-1'>
                  <div className='flex items-center gap-2'>
                    <p className='truncate text-sm font-medium'>{hostOf(task)}</p>
                    <Badge variant='destructive'>{task.fail_count}x</Badge>
                  </div>
                  <p className='text-muted-foreground truncate text-xs'>
                    {task.last_run_at ? formatTime(task.last_run_at) : t('dashboard.neverRan')} -{' '}
                    {task.last_message || t('dashboard.noDetail')}
                  </p>
                </div>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
