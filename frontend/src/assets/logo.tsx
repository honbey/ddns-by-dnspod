import { type SVGProps, useId } from 'react'
import { cn } from '@/lib/utils'

/**
 * The letters are DejaVu Sans Bold outlines, not text: a font would be whatever
 * the client happens to have, and the mark has to match the favicon pixel for
 * pixel. The transform maps the glyph box onto the counter of the D.
 */
const DNS_OUTLINES = [
  'M6.94-11.74L5.60-11.74L5.60-2.84L6.94-2.84Q9.25-2.84 10.46-3.98Q11.68-5.13 11.68-7.30Q11.68-9.47 10.47-10.61Q9.26-11.74 6.94-11.74M1.84 0L1.84-14.58L5.80-14.58Q9.12-14.58 10.75-14.11Q12.37-13.63 13.54-12.50Q14.56-11.51 15.06-10.22Q15.56-8.94 15.56-7.30Q15.56-5.65 15.06-4.36Q14.56-3.07 13.54-2.08Q12.36-0.95 10.72-0.47Q9.08 0 5.80 0',
  'M18.44 0L18.44-14.58L22.64-14.58L27.94-4.58L27.94-14.58L31.50-14.58L31.50 0L27.30 0L22-10L22 0',
  'M45.32-14.12L45.32-11.04Q44.12-11.57 42.98-11.85Q41.84-12.12 40.82-12.12Q39.47-12.12 38.83-11.75Q38.18-11.38 38.18-10.60Q38.18-10.01 38.62-9.68Q39.05-9.36 40.20-9.12L41.80-8.80Q44.23-8.31 45.25-7.31Q46.28-6.32 46.28-4.48Q46.28-2.07 44.85-0.89Q43.42 0.28 40.48 0.28Q39.09 0.28 37.70 0.02Q36.30-0.24 34.90-0.76L34.90-3.94Q36.30-3.19 37.60-2.82Q38.91-2.44 40.12-2.44Q41.35-2.44 42-2.85Q42.66-3.26 42.66-4.02Q42.66-4.71 42.21-5.08Q41.77-5.45 40.44-5.74L38.98-6.06Q36.80-6.53 35.79-7.56Q34.78-8.58 34.78-10.32Q34.78-12.50 36.18-13.67Q37.59-14.84 40.22-14.84Q41.43-14.84 42.70-14.66Q43.96-14.48 45.32-14.12',
]

/**
 * The mark is the favicon's hand-written D with DNS inside it.
 *
 * The letter is drawn as a single flowing outline - curled entry at the top
 * left, upper arc, right arc, closing baseline - because a geometric D would
 * disappear at the 32 px the sidebar uses. The label fills the counter, which
 * is what keeps the mark readable when scaled down.
 *
 * The tile keeps its own colour instead of `currentColor`: it has to read as
 * the app's stamp on either theme, in the sidebar and on the sign-in page.
 */
export function Logo({ className, ...props }: SVGProps<SVGSVGElement>) {
  // useId keeps the gradient unique when more than one Logo is on the page.
  // Colons are not valid inside a url(#) reference, so they are stripped.
  const gradientId = `logo-glow-${useId().replace(/[^a-zA-Z0-9_-]/g, '')}`

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
        <linearGradient
          id={gradientId}
          gradientUnits='userSpaceOnUse'
          x1='8'
          y1='8'
          x2='56'
          y2='56'
        >
          <stop offset='0' stopColor='#22d3ee' />
          <stop offset='1' stopColor='#3b82f6' />
        </linearGradient>
      </defs>

      <rect width='64' height='64' rx='14' fill='#0b1226' />

      <g fill='none' stroke={`url(#${gradientId})`} strokeLinecap='round' strokeLinejoin='round'>
        <path d='M20 14C28 8 42 8 50 17C58 26 57 43 46 51C40 55 31 57 23 57H11' strokeWidth='5.5' />
        <path d='M20 14C12 19 10 29 15 34' strokeWidth='5.5' />
      </g>

      <g
        transform='translate(23 26.44) scale(0.596 1) translate(-1.84 14.84)'
        fill={`url(#${gradientId})`}
      >
        {DNS_OUTLINES.map((outline) => (
          <path key={outline.slice(0, 12)} d={outline} />
        ))}
      </g>
    </svg>
  )
}
