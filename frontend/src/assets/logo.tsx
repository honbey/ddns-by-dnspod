import { type SVGProps } from 'react'
import { cn } from '@/lib/utils'

/**
 * The mark is a globe inside a rounded tile: the public address of this host,
 * kept pointed at the right place.
 *
 * The tile carries its own colour instead of `currentColor` on purpose - it has
 * to read as the app's stamp in the sidebar, on the sign-in page and in the tab
 * strip alike, so it does not follow the text colour around it. The globe is
 * stroked, not filled, which is what keeps it legible at 16-32 px.
 */
export function Logo({ className, ...props }: SVGProps<SVGSVGElement>) {
  return (
    <svg
      viewBox='0 0 64 64'
      xmlns='http://www.w3.org/2000/svg'
      className={cn('size-8', className)}
      role='img'
      {...props}
    >
      <title>DDNS by DNSPod</title>
      <defs>
        <linearGradient id='logo-tile' x1='0' y1='0' x2='1' y2='1'>
          <stop offset='0' stopColor='#334155' />
          <stop offset='1' stopColor='#0f172a' />
        </linearGradient>
      </defs>
      <rect width='64' height='64' rx='18' fill='url(#logo-tile)' />
      <g
        transform='translate(14 14) scale(1.5)'
        fill='none'
        stroke='#f8fafc'
        strokeWidth='1.75'
        strokeLinecap='round'
        strokeLinejoin='round'
      >
        <circle cx='12' cy='12' r='10' />
        <path d='M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z' />
        <line x1='2' y1='12' x2='22' y2='12' />
      </g>
    </svg>
  )
}
