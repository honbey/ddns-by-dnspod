type PageHeadingProps = {
  title: React.ReactNode
  /** One line under the title: what this page is, where it is, or what it holds. */
  subtitle?: React.ReactNode
  /** What can be done here, on the right of the title. */
  actions?: React.ReactNode
}

/**
 * The heading every page opens with: what the page is on the left, what someone
 * would do here on the right. One component so the pages do not drift apart -
 * the subtitle stays a single line, and the gap to what follows comes from the
 * `Main` around it.
 */
export function PageHeading({ title, subtitle, actions }: PageHeadingProps) {
  return (
    <div className='flex flex-wrap items-end justify-between gap-2'>
      <div>
        <h1 className='text-2xl font-bold tracking-tight'>{title}</h1>
        {subtitle ? <div className='text-muted-foreground text-sm'>{subtitle}</div> : null}
      </div>
      {actions ? <div className='flex items-center gap-2'>{actions}</div> : null}
    </div>
  )
}
