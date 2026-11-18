import { z } from 'zod'

/**
 * Parsers without the just-in-time compiler.
 *
 * Zod probes `new Function` on its first parse to decide whether it may compile
 * parsers at run time. The probe is caught, so nothing ever broke, but a strict
 * CSP still reports the attempt as a violation - and the fallback parser is what
 * a deployment with `script-src 'self'` ends up using anyway.
 *
 * Import this before anything that parses: the probe is remembered on first use.
 */
z.config({ jitless: true })
