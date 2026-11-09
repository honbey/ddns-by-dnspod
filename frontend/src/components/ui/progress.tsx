import { cn } from '@/lib/utils'

type ProgressProps = React.ComponentProps<'div'> & {
  value?: number
}

function Progress({ value = 0, className, ...props }: ProgressProps) {
  const clamped = Math.min(100, Math.max(0, value))

  return (
    <div
      data-slot='progress'
      role='progressbar'
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={clamped}
      className={cn('relative h-2 w-full overflow-hidden rounded-full bg-primary/20', className)}
      {...props}
    >
      <div
        data-slot='progress-indicator'
        className='h-full bg-primary transition-[width] duration-200'
        style={{ width: `${clamped}%` }}
      />
    </div>
  )
}

export { Progress }
