#!/usr/bin/env node
/**
 * Forwards a local TCP port to a loopback-only service.
 *
 * The server refuses to run with AUTH_DISABLED on a non-loopback address, so a
 * preview has to reach it through something that listens publicly on its
 * behalf. TCP passthrough keeps HTTP and the Vite/WebSocket upgrades working
 * without parsing either.
 *
 *   node scripts/forward.mjs
 *   node scripts/forward.mjs --listen 0.0.0.0:8080 --target 127.0.0.1:8081
 *   node scripts/forward.mjs --target 127.0.0.1:8082   # behind mock-dnspod
 *
 * LISTEN and TARGET carry the same values when the flags are absent. Anything
 * that is not loopback is announced, because the demo server behind it has no
 * authentication at all.
 */
import net from 'node:net'

function parseArgs(argv) {
  const out = {}
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i]
    if (arg !== '--listen' && arg !== '--target') {
      throw new Error(`unknown argument: ${arg}`)
    }
    const value = argv[i + 1]
    if (!value || value.startsWith('--')) throw new Error(`${arg} needs a value`)
    out[arg.slice(2)] = value
    i += 1
  }
  return out
}

function endpoint(value, defaultHost) {
  let host
  let portText
  if (value.startsWith('[')) {
    const end = value.indexOf(']')
    if (end < 0 || value[end + 1] !== ':') throw new Error(`invalid address: ${value}`)
    host = value.slice(1, end)
    portText = value.slice(end + 2)
  } else {
    const at = value.lastIndexOf(':')
    if (at < 0) {
      host = defaultHost
      portText = value
    } else {
      host = value.slice(0, at)
      portText = value.slice(at + 1)
    }
  }

  const port = Number(portText)
  if (!host) throw new Error(`invalid address: ${value}`)
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    throw new Error(`invalid port in ${value}`)
  }
  return [host, port]
}

let listen
let target
try {
  const args = parseArgs(process.argv.slice(2))
  listen = endpoint(args.listen ?? process.env.LISTEN ?? '0.0.0.0:8080', '0.0.0.0')
  target = endpoint(args.target ?? process.env.TARGET ?? '127.0.0.1:8081', '127.0.0.1')
} catch (err) {
  console.error(`forward: ${err.message}`)
  process.exit(1)
}

const [listenHost, listenPort] = listen
const [targetHost, targetPort] = target

const isLoopback =
  listenHost === 'localhost' || listenHost === '::1' || listenHost.startsWith('127.')
if (!isLoopback) {
  console.warn(
    `forward: ${listenHost}:${listenPort} is not loopback; the demo server has no authentication`
  )
}

const server = net.createServer((client) => {
  const upstream = net.connect({ host: targetHost, port: targetPort })

  const close = () => {
    client.destroy()
    upstream.destroy()
  }

  client.on('error', close)
  upstream.on('error', close)
  client.pipe(upstream)
  upstream.pipe(client)
})

server.on('error', (err) => {
  console.error(`forward: ${err.message}`)
  process.exit(1)
})

server.listen(listenPort, listenHost, () => {
  console.log(`forward: ${listenHost}:${listenPort} -> ${targetHost}:${targetPort}`)
})
