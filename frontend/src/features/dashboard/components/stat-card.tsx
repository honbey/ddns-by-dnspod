import { type LucideIcon } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

type StatCardProps = {
  title: string
  value: number | string
  icon: LucideIcon
  loading?: boolean
}

/**
 * One number, with the label above it and the icon opposite.
 *
 * Nothing under the number on purpose: a caption there competes with it for
 * attention and rarely says more than the page it links to already does.
 */
export function StatCard({ title, value, icon: Icon, loading }: StatCardProps) {
  return (
    <Card>
      <CardHeader className='flex flex-row items-center justify-between gap-2 space-y-0'>
        <CardTitle className='text-sm font-medium'>{title}</CardTitle>
        <Icon aria-hidden='true' className='text-muted-foreground size-4' />
      </CardHeader>
      <CardContent>
        {loading ? (
          <Skeleton className='h-9 w-16' />
        ) : (
          <div className='text-3xl font-bold tracking-tight'>{value}</div>
        )}
      </CardContent>
    </Card>
  )
}
