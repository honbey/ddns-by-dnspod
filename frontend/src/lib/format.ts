/** Formatting shared by the pages and the tables. */

/**
 * A duration in seconds, in the units a person would say it in.
 *
 * Two units at most: "6h 20m" rather than "6h 20m 5s". Kept here rather than in
 * the chart because both the axis labels and the cells use it, and they have to
 * agree or a tooltip contradicts the scale next to it.
 */
export function formatDuration(sec?: number | null): string {
  if (sec == null || !Number.isFinite(sec)) return '-'
  if (sec < 60) return `${Math.round(sec)}s`
  if (sec < 3600) {
    const m = Math.floor(sec / 60)
    const s = Math.round(sec % 60)
    return s ? `${m}m ${s}s` : `${m}m`
  }
  if (sec < 86400) {
    const h = Math.floor(sec / 3600)
    const m = Math.floor((sec % 3600) / 60)
    return m ? `${h}h ${m}m` : `${h}h`
  }
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  return h ? `${d}d ${h}h` : `${d}d`
}

/** Date and time of a timestamp, in the viewer's locale. */
export function formatTime(value?: string | null): string {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(date)
}

/** Maps a run status onto a badge variant. */
export function statusVariant(status: string): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case 'OK':
      return 'default'
    case 'FAIL':
      return 'destructive'
    case 'SKIP':
    case 'RUNNING':
      return 'secondary'
    default:
      return 'outline'
  }
}
