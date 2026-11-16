#!/usr/bin/env node
/**
 * A read-only stand-in for the DNSPod-backed console endpoints.
 *
 * The server talks to the real DNSPod API, so a demo with fake credentials
 * makes the domain and record pages fail. This proxy sits in front of it: the
 * handful of read endpoints those pages use answer from fixtures, everything
 * else is forwarded untouched. Writes still reach the server, so they keep
 * behaving exactly as they would against fake credentials.
 *
 *   node scripts/mock-dnspod.mjs
 *   node scripts/mock-dnspod.mjs --listen 127.0.0.1:8082 --target 127.0.0.1:8081
 *
 * In the demo stack the forwarder points at this proxy:
 *   forward --listen 0.0.0.0:8080 --target 127.0.0.1:8082
 */
import http from 'node:http'

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

function endpoint(value) {
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
      host = '127.0.0.1'
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
  listen = endpoint(args.listen ?? process.env.MOCK_LISTEN ?? '127.0.0.1:8082')
  target = endpoint(args.target ?? process.env.MOCK_TARGET ?? '127.0.0.1:8081')
} catch (err) {
  console.error(`mock-dnspod: ${err.message}`)
  process.exit(1)
}

const [listenHost, listenPort] = listen
const [targetHost, targetPort] = target

/** The host's real address keeps the records in step with the dashboard card. */
async function liveIPv4() {
  try {
    const res = await fetch(`http://${targetHost}:${targetPort}/api/ip?family=v4`, {
      signal: AbortSignal.timeout(3000),
    })
    const body = await res.json()
    return body?.data?.available ? body.data.ip : null
  } catch {
    return null
  }
}

const ipv4 = (await liveIPv4()) ?? '203.0.113.10'
const ipv6 = '2402:4e00:1010::1'

const domains = [
  { domain_id: 901, name: 'example.com', status: 'ENABLE', ttl: 600, grade: 'DP_Plus', records: 8, group_id: 1 },
  { domain_id: 902, name: 'example.net', status: 'ENABLE', ttl: 600, grade: 'DP_Free', records: 3, group_id: 1 },
  { domain_id: 903, name: 'lab.example.net', status: 'ENABLE', ttl: 600, grade: 'DP_Free', records: 3, group_id: 2 },
]

const domainGroups = [
  { id: 1, name: '生产', size: 2 },
  { id: 2, name: '测试', size: 1 },
]

const lines = [
  { name: '默认', id: '0' },
  { name: '电信', id: '10' },
  { name: '联通', id: '20' },
  { name: '移动', id: '30' },
  { name: '境外', id: '40' },
]

const recordGroups = [
  { id: 1, name: 'DDNS', type: 'custom' },
  { id: 2, name: '邮件', type: 'custom' },
]

function record(recordId, subDomain, recordType, value, line = '默认', extra = {}) {
  return {
    record_id: recordId,
    sub_domain: subDomain,
    record_type: recordType,
    value,
    line,
    line_id: lines.find((item) => item.name === line)?.id ?? '0',
    ttl: 600,
    mx: 0,
    weight: 0,
    status: 'ENABLE',
    remark: '',
    updated_on: '2026-11-16 20:30:00',
    ...extra,
  }
}

const recordsByDomain = {
  'example.com': [
    record(101, 'home', 'A', ipv4),
    record(102, 'nas', 'A', ipv4),
    record(103, 'www', 'A', ipv4, '电信'),
    record(104, 'api', 'A', '42.193.160.12'),
    record(105, 'vault', 'AAAA', ipv6),
    record(106, '@', 'MX', 'mx.example.com', '默认', { mx: 10 }),
    record(107, 'blog', 'CNAME', 'example.com'),
    record(108, '@', 'TXT', 'v=spf1 include:_spf.example.com -all'),
  ],
  'example.net': [
    record(201, 'git', 'A', ipv4),
    record(202, 'www', 'CNAME', 'example.net'),
    record(203, '@', 'MX', 'mx.example.net', '默认', { mx: 10 }),
  ],
  'lab.example.net': [
    record(301, 'git', 'A', '203.0.113.21'),
    record(302, 'build', 'A', '203.0.113.22'),
    record(303, '@', 'TXT', 'demo lab'),
  ],
}

function pagedRecords(domain, query) {
  const all = recordsByDomain[domain] ?? []
  const subDomain = query.get('sub_domain')?.trim() ?? ''
  const recordType = query.get('record_type')?.trim() ?? ''
  const keyword = query.get('keyword')?.trim().toLowerCase() ?? ''

  const filtered = all.filter((item) => {
    if (subDomain && item.sub_domain !== subDomain) return false
    if (recordType && item.record_type !== recordType) return false
    if (keyword && !`${item.sub_domain} ${item.value}`.toLowerCase().includes(keyword)) return false
    return true
  })

  const page = Math.max(1, Number(query.get('page') ?? 1) || 1)
  const pageSize = Math.min(200, Math.max(1, Number(query.get('page_size') ?? 20) || 20))
  const start = (page - 1) * pageSize
  return {
    items: filtered.slice(start, start + pageSize),
    total: filtered.length,
    page,
    page_size: pageSize,
  }
}

function fixture(url) {
  if (url.pathname === '/api/domains') return domains
  if (url.pathname === '/api/domain-groups') return domainGroups

  const match = /^\/api\/domains\/([^/]+)\/(records|lines|groups)$/.exec(url.pathname)
  if (!match) return null

  const domain = decodeURIComponent(match[1])
  if (match[2] === 'records') return pagedRecords(domain, url.searchParams)
  if (match[2] === 'lines') return lines
  return recordGroups
}

function sendJSON(res, body) {
  const payload = JSON.stringify({ data: body })
  res.writeHead(200, {
    'content-type': 'application/json; charset=utf-8',
    'cache-control': 'no-store',
    'content-length': Buffer.byteLength(payload),
  })
  res.end(payload)
}

function forward(req, res) {
  const headers = { ...req.headers, host: `${targetHost}:${targetPort}` }
  delete headers.connection

  const upstream = http.request(
    { host: targetHost, port: targetPort, method: req.method, path: req.url, headers },
    (upstreamRes) => {
      res.writeHead(upstreamRes.statusCode ?? 502, upstreamRes.headers)
      upstreamRes.pipe(res)
    }
  )

  upstream.on('error', (err) => {
    if (res.writableEnded) return
    const payload = JSON.stringify({ error: 'mock upstream unreachable', detail: err.message })
    res.writeHead(502, {
      'content-type': 'application/json; charset=utf-8',
      'content-length': Buffer.byteLength(payload),
    })
    res.end(payload)
  })

  req.pipe(upstream)
}

function handle(req, res) {
  const url = new URL(req.url ?? '/', `http://${req.headers.host ?? 'localhost'}`)

  if (req.method === 'GET') {
    const body = fixture(url)
    if (body !== null) {
      sendJSON(res, body)
      return
    }
  }

  forward(req, res)
}

const server = http.createServer((req, res) => {
  try {
    handle(req, res)
  } catch (err) {
    console.error(`mock-dnspod: request failed: ${err.message}`)
    if (res.headersSent) {
      res.destroy()
      return
    }
    const payload = JSON.stringify({ error: 'bad request' })
    res.writeHead(400, {
      'content-type': 'application/json; charset=utf-8',
      'content-length': Buffer.byteLength(payload),
    })
    res.end(payload)
  }
})

server.on('error', (err) => {
  console.error(`mock-dnspod: ${err.message}`)
  process.exit(1)
})

server.listen(listenPort, listenHost, () => {
  console.log(
    `mock-dnspod: http://${listenHost}:${listenPort} -> http://${targetHost}:${targetPort} (fixtures answer without authentication)`
  )
})
