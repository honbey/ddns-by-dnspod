#!/usr/bin/env node
/**
 * Seeds a demo SQLite database so the console can be shown with data.
 *
 * The demo tables are cleared and rewritten, so never point this at a
 * deployment that matters. The server has to have created the schema once:
 * start it against the same database before running this.
 *
 *   node scripts/mock-data.mjs
 *   node scripts/mock-data.mjs --db /tmp/demo/ddns.db --key /tmp/demo/secret.key
 *
 * The database defaults to DDNS_DATABASE_PATH (data/ddns.db). The key follows
 * the server: DDNS_CRYPTO_KEY (base64 32 bytes) wins over the key file, which
 * defaults to secret.key next to the database. The key must already exist;
 * starting the server against this database once creates it.
 *
 * The seeded tasks are enabled, so run the demo server with
 * DDNS_SCHEDULER_ENABLED=false: otherwise they try the fake credentials and
 * overwrite the curated statuses with failures.
 */
import { createCipheriv, randomBytes } from 'node:crypto'
import { existsSync, readFileSync } from 'node:fs'
import path from 'node:path'
import { DatabaseSync } from 'node:sqlite'

function parseArgs(argv) {
  const out = {}
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i]
    if (arg !== '--db' && arg !== '--key') throw new Error(`unknown argument: ${arg}`)
    const value = argv[i + 1]
    if (!value || value.startsWith('--')) throw new Error(`${arg} needs a value`)
    out[arg.slice(2)] = value
    i += 1
  }
  return out
}

function fail(message) {
  console.error(`mock-data: ${message}`)
  process.exit(1)
}

let args
try {
  args = parseArgs(process.argv.slice(2))
} catch (err) {
  console.error(`mock-data: ${err.message}`)
  process.exit(1)
}

const dbPath = args.db ?? process.env.DDNS_DATABASE_PATH ?? 'data/ddns.db'
const keyPath =
  args.key ?? process.env.DDNS_CRYPTO_KEY_FILE ?? path.join(path.dirname(dbPath), 'secret.key')

/**
 * The server stores credentials as base64(nonce || AES-256-GCM ciphertext ||
 * tag), keyed by the base64 32-byte key the deployment generated on first
 * start. A plain string here would fail to decrypt and the account would answer
 * 500 on every upstream call.
 */
function keyFromEnv() {
  const raw = process.env.DDNS_CRYPTO_KEY
  if (!raw) return null
  const key = Buffer.from(raw.trim(), 'base64')
  if (key.length !== 32) {
    throw new Error(`DDNS_CRYPTO_KEY should decode to 32 bytes, got ${key.length}`)
  }
  return key
}

function loadCryptoKey(file) {
  if (!existsSync(file)) {
    throw new Error(`key file ${file} not found; start the server once, pass --key, or set DDNS_CRYPTO_KEY`)
  }
  const key = Buffer.from(readFileSync(file, 'utf8').trim(), 'base64')
  if (key.length !== 32) {
    throw new Error(`${file} should hold a base64 32-byte key, got ${key.length} bytes`)
  }
  return key
}

function sealSecret(plain, key) {
  const nonce = randomBytes(12)
  const cipher = createCipheriv('aes-256-gcm', key, nonce)
  const ciphertext = Buffer.concat([cipher.update(plain, 'utf8'), cipher.final()])
  return Buffer.concat([nonce, ciphertext, cipher.getAuthTag()]).toString('base64')
}

const TZ_OFFSET_MS = 8 * 60 * 60 * 1000

/** SQLite datetime GORM can scan: YYYY-MM-DD HH:MM:SS+08:00 */
function ts(deltaMs = 0) {
  const d = new Date(Date.now() + deltaMs + TZ_OFFSET_MS)
  return `${d.toISOString().slice(0, 19).replace('T', ' ')}+08:00`
}

const MINUTE = 60 * 1000
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR

/** The address the preview host reports right now, so the cards agree with the tasks. */
async function liveIPv4() {
  try {
    const port = process.env.PREVIEW_PORT ?? '8080'
    const res = await fetch(`http://127.0.0.1:${port}/api/ip?family=v4`, {
      signal: AbortSignal.timeout(3000),
    })
    const body = await res.json()
    return body?.data?.available ? body.data.ip : null
  } catch {
    return null
  }
}

if (!existsSync(dbPath)) {
  fail(`${dbPath} does not exist; start the server against it once before seeding`)
}

const db = new DatabaseSync(dbPath)
db.exec('PRAGMA foreign_keys = ON')
db.exec('PRAGMA busy_timeout = 5000')

const tables = new Set(
  db.prepare("SELECT name FROM sqlite_master WHERE type = 'table'").all().map((row) => row.name)
)
if (!tables.has('accounts')) {
  fail(`${dbPath} has no schema yet; start the server against it once before seeding`)
}

if (!process.env.DDNS_CRYPTO_KEY && !existsSync(keyPath)) {
  fail(`key file ${keyPath} not found; start the server once, pass --key, or set DDNS_CRYPTO_KEY`)
}

const currentV4 = (await liveIPv4()) ?? '42.193.175.155'
const cryptoKey = keyFromEnv() ?? loadCryptoKey(keyPath)

const accounts = [
  {
    id: 1,
    name: '主账号',
    secretId: 'AKIDdemo000000000000000000000001',
    secretPlain: 'demo-secret-key-0001',
    keyHint: 'AbC******xYz',
    isDefault: 1,
    createdAt: ts(-400 * DAY),
  },
  {
    id: 2,
    name: '备用账号',
    secretId: 'AKIDdemo000000000000000000000002',
    secretPlain: 'demo-secret-key-0002',
    keyHint: 'Def******uVw',
    isDefault: 0,
    createdAt: ts(-220 * DAY),
  },
]

const tasks = [
  {
    id: 1,
    accountId: 1,
    domain: 'example.com',
    subDomain: 'home',
    recordType: 'A',
    line: '默认',
    ttl: 600,
    enabled: 1,
    cron: '*/5 * * * *',
    currentIp: currentV4,
    ipSince: ts(-3 * DAY),
    lastRunAt: ts(-4 * MINUTE),
    lastStatus: 'OK',
    lastMessage: '',
    failCount: 0,
    notify: 1,
    createdAt: ts(-120 * DAY),
  },
  {
    id: 2,
    accountId: 1,
    domain: 'example.com',
    subDomain: 'nas',
    recordType: 'A',
    line: '默认',
    ttl: 600,
    enabled: 1,
    cron: '*/10 * * * *',
    currentIp: currentV4,
    ipSince: ts(-3 * DAY),
    lastRunAt: ts(-7 * MINUTE),
    lastStatus: 'OK',
    lastMessage: '',
    failCount: 0,
    notify: 1,
    createdAt: ts(-120 * DAY),
  },
  {
    id: 3,
    accountId: 1,
    domain: 'example.com',
    subDomain: 'www',
    recordType: 'A',
    line: '电信',
    ttl: 600,
    enabled: 1,
    cron: '*/5 * * * *',
    currentIp: currentV4,
    ipSince: ts(-3 * DAY),
    lastRunAt: ts(-2 * MINUTE),
    lastStatus: 'OK',
    lastMessage: '',
    failCount: 0,
    notify: 0,
    createdAt: ts(-90 * DAY),
  },
  {
    id: 4,
    accountId: 1,
    domain: 'example.com',
    subDomain: 'vault',
    recordType: 'AAAA',
    line: '默认',
    ttl: 600,
    enabled: 1,
    cron: '*/10 * * * *',
    currentIp: '2402:4e00:1010::1',
    ipSince: ts(-4 * DAY),
    lastRunAt: ts(-9 * MINUTE),
    lastStatus: 'OK',
    lastMessage: '',
    failCount: 0,
    notify: 1,
    createdAt: ts(-60 * DAY),
  },
  {
    id: 5,
    accountId: 1,
    domain: 'example.com',
    subDomain: 'api',
    recordType: 'A',
    line: '默认',
    ttl: 120,
    enabled: 1,
    cron: '*/1 * * * *',
    currentIp: '42.193.160.12',
    ipSince: ts(-6 * DAY),
    lastRunAt: ts(-3 * MINUTE),
    lastStatus: 'FAIL',
    lastMessage: 'DNSPod API error: 请求频率超出限制',
    failCount: 3,
    notify: 1,
    createdAt: ts(-45 * DAY),
  },
  {
    id: 6,
    accountId: 2,
    domain: 'lab.example.net',
    subDomain: 'git',
    recordType: 'A',
    line: '默认',
    ttl: 600,
    enabled: 0,
    cron: '0 3 * * *',
    currentIp: '',
    ipSince: null,
    lastRunAt: ts(-40 * DAY),
    lastStatus: 'OK',
    lastMessage: '',
    failCount: 0,
    notify: 0,
    createdAt: ts(-40 * DAY),
  },
]

const history = [
  { taskId: 1, domain: 'example.com', sub: 'home', type: 'A', oldIp: '42.193.168.10', newIp: '42.193.160.12', status: 'OK', message: '', duration: 12 * 86400, at: ts(-6 * DAY) },
  { taskId: 1, domain: 'example.com', sub: 'home', type: 'A', oldIp: '42.193.160.12', newIp: currentV4, status: 'OK', message: '', duration: 3 * 86400, at: ts(-3 * DAY) },
  { taskId: 2, domain: 'example.com', sub: 'nas', type: 'A', oldIp: '42.193.168.10', newIp: '42.193.160.12', status: 'OK', message: '', duration: 12 * 86400, at: ts(-6 * DAY) },
  { taskId: 2, domain: 'example.com', sub: 'nas', type: 'A', oldIp: '42.193.160.12', newIp: currentV4, status: 'OK', message: '', duration: 3 * 86400, at: ts(-3 * DAY) },
  { taskId: 3, domain: 'example.com', sub: 'www', type: 'A', oldIp: '42.193.160.12', newIp: currentV4, status: 'OK', message: '', duration: 3 * 86400, at: ts(-3 * DAY) },
  { taskId: 4, domain: 'example.com', sub: 'vault', type: 'AAAA', oldIp: '2402:4e00:1010::9', newIp: '2402:4e00:1010::1', status: 'OK', message: '', duration: 16 * 86400, at: ts(-4 * DAY) },
  { taskId: 5, domain: 'example.com', sub: 'api', type: 'A', oldIp: '42.193.160.12', newIp: '', status: 'FAIL', message: 'DNSPod API error: 请求频率超出限制', duration: null, at: ts(-2 * DAY) },
  { taskId: 5, domain: 'example.com', sub: 'api', type: 'A', oldIp: '42.193.160.12', newIp: '', status: 'FAIL', message: 'DNSPod API error: 请求频率超出限制', duration: null, at: ts(-26 * HOUR) },
  { taskId: 6, domain: 'lab.example.net', sub: 'git', type: 'A', oldIp: '203.0.113.20', newIp: '203.0.113.21', status: 'OK', message: '', duration: 40 * 86400, at: ts(-40 * DAY) },
  { taskId: 3, domain: 'example.com', sub: 'www', type: 'A', oldIp: '42.193.150.7', newIp: '42.193.168.10', status: 'OK', message: '', duration: 12 * 86400, at: ts(-18 * DAY) },
]

const ipChanges = [
  { family: 'v4', oldIp: '', newIp: '42.193.150.7', duration: null, at: ts(-30 * DAY) },
  { family: 'v4', oldIp: '42.193.150.7', newIp: '42.193.168.10', duration: 12 * 86400, at: ts(-18 * DAY) },
  { family: 'v4', oldIp: '42.193.168.10', newIp: '42.193.160.12', duration: 12 * 86400, at: ts(-6 * DAY) },
  { family: 'v4', oldIp: '42.193.160.12', newIp: currentV4, duration: 3 * 86400, at: ts(-3 * DAY) },
  { family: 'v6', oldIp: '', newIp: '2402:4e00:1010::9', duration: null, at: ts(-20 * DAY) },
  { family: 'v6', oldIp: '2402:4e00:1010::9', newIp: '2402:4e00:1010::1', duration: 16 * 86400, at: ts(-4 * DAY) },
]

const certificates = [
  {
    id: 1,
    name: 'home-wildcard',
    accountId: 1,
    domains: ['*.example.com', 'example.com'],
    keyType: 'ec256',
    outputDir: 'home',
    renewCron: '0 3 * * *',
    renewBeforeDays: 30,
    notifyOnIssue: 1,
    lastIssuedAt: ts(-40 * DAY),
    notAfter: ts(50 * DAY),
    lastStatus: 'OK',
    lastMessage: '',
    failCount: 0,
    createdAt: ts(-200 * DAY),
  },
  {
    id: 2,
    name: 'api-cert',
    accountId: 2,
    domains: ['api.example.net'],
    keyType: 'rsa2048',
    outputDir: 'api',
    renewCron: '0 4 * * *',
    renewBeforeDays: 14,
    notifyOnIssue: 0,
    lastIssuedAt: ts(-78 * DAY),
    notAfter: ts(12 * DAY),
    lastStatus: 'OK',
    lastMessage: '',
    failCount: 0,
    createdAt: ts(-150 * DAY),
  },
]

const channels = [
  {
    id: 1,
    name: 'ntfy 推送',
    type: 'ntfy',
    enabled: 1,
    config: { server: 'https://ntfy.sh', topic: 'ddns-demo-home', priority: 3, tags: 'house' },
    createdAt: ts(-100 * DAY),
  },
  {
    id: 2,
    name: '企业微信',
    type: 'webhook',
    enabled: 0,
    config: { url: 'https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=demo', method: 'POST' },
    createdAt: ts(-80 * DAY),
  },
]

const apiKeys = [
  { id: 1, name: 'Home Assistant', prefix: 'ddns_live_9f2a', domain: 'example.com', accountId: 1, enabled: 1, lastUsedAt: ts(-2 * HOUR) },
  { id: 2, name: 'NAS 备份', prefix: 'ddns_live_3c71', domain: '*', accountId: 1, enabled: 1, lastUsedAt: ts(-5 * DAY) },
  { id: 3, name: '旧脚本', prefix: 'ddns_live_77be', domain: 'lab.example.net', accountId: 2, enabled: 0, lastUsedAt: null },
]

db.exec('BEGIN')
try {
  for (const table of [
    'ddns_history',
    'public_ip_changes',
    'certificates',
    'api_keys',
    'notify_channels',
    'ddns_tasks',
    'accounts',
  ]) {
    db.exec(`DELETE FROM ${table}`)
  }
  db.exec(
    "DELETE FROM sqlite_sequence WHERE name IN ('ddns_history','public_ip_changes','certificates','api_keys','notify_channels','ddns_tasks','accounts')"
  )

  const insertAccount = db.prepare(
    'INSERT INTO accounts (id,name,secret_id,secret_key,key_hint,is_default,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?)'
  )
  for (const a of accounts) {
    insertAccount.run(a.id, a.name, a.secretId, sealSecret(a.secretPlain, cryptoKey), a.keyHint, a.isDefault, a.createdAt, a.createdAt)
  }

  const insertTask = db.prepare(
    `INSERT INTO ddns_tasks
      (id,account_id,domain,sub_domain,record_type,record_line,ttl,enabled,cron_expr,current_ip,ip_since,last_run_at,last_status,last_message,fail_count,notify_on_change,created_at,updated_at)
     VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
  )
  for (const t of tasks) {
    insertTask.run(
      t.id,
      t.accountId,
      t.domain,
      t.subDomain,
      t.recordType,
      t.line,
      t.ttl,
      t.enabled,
      t.cron,
      t.currentIp,
      t.ipSince,
      t.lastRunAt,
      t.lastStatus,
      t.lastMessage,
      t.failCount,
      t.notify,
      t.createdAt,
      t.createdAt
    )
  }

  const insertHistory = db.prepare(
    `INSERT INTO ddns_history
      (task_id,domain,sub_domain,record_type,record_id,old_ip,new_ip,status,message,duration_sec,created_at)
     VALUES (?,?,?,?,?,?,?,?,?,?,?)`
  )
  for (const h of history) {
    insertHistory.run(h.taskId, h.domain, h.sub, h.type, null, h.oldIp, h.newIp, h.status, h.message, h.duration, h.at)
  }

  const insertChange = db.prepare(
    'INSERT INTO public_ip_changes (family,old_ip,new_ip,duration_sec,created_at) VALUES (?,?,?,?,?)'
  )
  for (const c of ipChanges) {
    insertChange.run(c.family, c.oldIp, c.newIp, c.duration, c.at)
  }

  const insertCertificate = db.prepare(
    `INSERT INTO certificates
      (id,name,account_id,domains,key_type,output_dir,renew_cron,renew_before_days,notify_on_issue,last_issued_at,not_after,last_status,last_message,fail_count,created_at,updated_at)
     VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
  )
  for (const c of certificates) {
    insertCertificate.run(
      c.id,
      c.name,
      c.accountId,
      JSON.stringify(c.domains),
      c.keyType,
      c.outputDir,
      c.renewCron,
      c.renewBeforeDays,
      c.notifyOnIssue,
      c.lastIssuedAt,
      c.notAfter,
      c.lastStatus,
      c.lastMessage,
      c.failCount,
      c.createdAt,
      c.createdAt
    )
  }

  const insertChannel = db.prepare(
    'INSERT INTO notify_channels (id,name,type,enabled,config,created_at,updated_at) VALUES (?,?,?,?,?,?,?)'
  )
  for (const c of channels) {
    insertChannel.run(c.id, c.name, c.type, c.enabled, JSON.stringify(c.config), c.createdAt, c.createdAt)
  }

  const insertKey = db.prepare(
    'INSERT INTO api_keys (id,name,key_hash,key_prefix,domain,account_id,enabled,last_used_at,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?)'
  )
  for (const k of apiKeys) {
    insertKey.run(
      k.id,
      k.name,
      `demo-hash-${k.id}`.padEnd(64, '0').slice(0, 64),
      k.prefix,
      k.domain,
      k.accountId,
      k.enabled,
      k.lastUsedAt,
      ts(-90 * DAY),
      ts(-90 * DAY)
    )
  }

  db.exec('COMMIT')
} catch (err) {
  db.exec('ROLLBACK')
  throw err
}

const count = (table) => db.prepare(`SELECT COUNT(*) AS n FROM ${table}`).get().n
console.log(
  `seeded ${dbPath}: accounts=${count('accounts')} tasks=${count('ddns_tasks')} history=${count('ddns_history')} ` +
    `ip_changes=${count('public_ip_changes')} certificates=${count('certificates')} ` +
    `channels=${count('notify_channels')} api_keys=${count('api_keys')}`
)

db.close()
