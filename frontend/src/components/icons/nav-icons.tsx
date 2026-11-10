import { type SVGProps } from 'react'
import { Languages } from 'lucide-react'

/**
 * Glyphs for the sidebar and the dashboard where lucide has no close match.
 *
 * The semantics come from Material Design Icons, the rendering does not: these
 * are drawn on the same 24x24 grid with 2px round strokes and no fill, so a row
 * mixing them with lucide keeps one weight and one rhythm.
 */
type GlyphProps = SVGProps<SVGSVGElement>

function Glyph({ children, ...props }: GlyphProps) {
  return (
    <svg
      xmlns='http://www.w3.org/2000/svg'
      viewBox='0 0 24 24'
      fill='none'
      stroke='currentColor'
      strokeWidth={2}
      strokeLinecap='round'
      strokeLinejoin='round'
      aria-hidden='true'
      {...props}
    >
      {children}
    </svg>
  )
}

/** A certificate: the sheet it is printed on, its star seal, and the ribbon. */
export function CertificateIcon(props: GlyphProps) {
  return (
    <Glyph {...props}>
      <rect x='3' y='4' width='18' height='12' rx='2' />
      {/* Five points this small close up under a 2px stroke, so the seal alone
          takes a thinner line to keep its notches readable. */}
      <path
        strokeWidth={1.5}
        d='M12 6.6 12.85 8.83 15.23 8.95 13.38 10.45 14 12.75 12 11.45 10 12.75 10.62 10.45 8.77 8.95 11.15 8.83Z'
      />
      <path d='M9 16v5l3-2 3 2v-5' />
    </Glyph>
  )
}

/** A domain: a window with a sidebar and its list of entries. */
export function DomainIcon(props: GlyphProps) {
  return (
    <Glyph {...props}>
      <rect x='3' y='4' width='18' height='16' rx='2' />
      <path d='M3 9h18' />
      <path d='M10 9v11' />
      <path d='M13 13h5M13 16h5' />
    </Glyph>
  )
}

/** An account: the provider's cloud, with the key that unlocks it. */
export function AccountIcon(props: GlyphProps) {
  return (
    // A tighter viewBox scales the cloud up about 11%; the thinner stroke
    // keeps the same 2px line weight as the lucide glyphs around it.
    <Glyph viewBox='1.2 1.2 21.6 21.6' strokeWidth={1.8} {...props}>
      <path d='M7 18a4 4 0 0 1 .6-7.96A5.5 5.5 0 0 1 18 9.5a3.5 3.5 0 0 1 .4 6.95' />
      <circle cx='14.5' cy='16' r='1.6' />
      <path d='M15.7 17.2 19 20.5M18 19.5l-1 1' />
    </Glyph>
  )
}

/** Translation. lucide's own mark is the same two-letter idea, already drawn. */
export function TranslateIcon(props: GlyphProps) {
  return <Languages {...props} />
}

/**
 * A node linked to two others, with the family lettered underneath. The frame
 * is deliberately gone: an address is a point in a network, not a machine.
 */
function IpIcon({ family, ...props }: GlyphProps & { family: 'v4' | 'v6' }) {
  return (
    <Glyph {...props}>
      <circle cx='5' cy='3.5' r='1.9' />
      <circle cx='19' cy='3.5' r='1.9' />
      <circle cx='12' cy='8.5' r='1.9' />
      <path d='M6.4 4.6 10.5 7.5M17.6 4.6 13.5 7.5' />
      <text
        x='12'
        y='22.5'
        textAnchor='middle'
        fontSize='9.5'
        fontWeight='700'
        fill='currentColor'
        stroke='none'
      >
        {family}
      </text>
    </Glyph>
  )
}

export function IpV4Icon(props: GlyphProps) {
  return <IpIcon family='v4' {...props} />
}

export function IpV6Icon(props: GlyphProps) {
  return <IpIcon family='v6' {...props} />
}
