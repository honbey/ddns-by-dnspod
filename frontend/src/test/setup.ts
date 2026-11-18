import { Storage } from 'happy-dom'

/**
 * Node 26 exposes its own Web Storage globals. `localStorage` reads as
 * undefined unless the process was started with `--localstorage-file`, and
 * either way the globals shadow the ones happy-dom would install. Tests expect
 * a browser, so replace both with a working Storage.
 */
for (const name of ['localStorage', 'sessionStorage'] as const) {
  Object.defineProperty(globalThis, name, {
    configurable: true,
    value: new Storage(),
  })
}
