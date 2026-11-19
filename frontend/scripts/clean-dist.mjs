#!/usr/bin/env node
/**
 * Empties the Go embed directory before Vite writes into it, keeping the
 * placeholder that lets `go test`/`go build` work from a checkout that has not
 * built the SPA yet.
 *
 * Vite's `emptyOutDir` removes `.gitkeep` along with the old assets. The build
 * disables that and runs this instead: stale files still go away, while the
 * embed pattern always has a directory to match.
 */
import { mkdir, readdir, rm, writeFile } from 'node:fs/promises'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = join(dirname(fileURLToPath(import.meta.url)), '..')
const outDir = resolve(root, process.env.BUILD_OUT_DIR || '../backend/internal/frontend/dist')

await mkdir(outDir, { recursive: true })
for (const entry of await readdir(outDir)) {
  if (entry === '.gitkeep') continue
  await rm(join(outDir, entry), { recursive: true, force: true })
}
await writeFile(join(outDir, '.gitkeep'), '')
