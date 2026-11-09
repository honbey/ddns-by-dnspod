// Wire types of the Go API.
//
// These are aliases of `backend/openapi.yml`, which is the only place a
// payload shape is written: openapi-typescript turns it into
// `api-types.gen.ts` (see `npm run types`) and this module gives the
// application its own names. Nothing here is invented - a field that is not in
// the document does not compile.
import type { components } from '@/api-types.gen'

export type Account = components['schemas']['Account']
export type Domain = components['schemas']['Domain']
export type DnsRecord = components['schemas']['DnsRecord']
export type Line = components['schemas']['Line']
export type Group = components['schemas']['Group']
export type ImportResult = components['schemas']['ImportResult']
export type DDNSTask = components['schemas']['DDNSTask']
export type HistoryEntry = components['schemas']['HistoryEntry']
export type IPChange = components['schemas']['IPChange']
export type IPChangePoint = components['schemas']['IPChangePoint']
export type IPHistoryImportResult = components['schemas']['IPHistoryImportResult']
export type Certificate = components['schemas']['Certificate']
export type NotifyChannel = components['schemas']['NotifyChannel']
export type RunResult = components['schemas']['RunResult']
export type BatchCreateResult = components['schemas']['BatchCreateResult']
export type BatchDeleteResult = components['schemas']['BatchDeleteResult']
export type PublicIP = components['schemas']['PublicIP']
export type APIKey = components['schemas']['APIKey']
export type APIKeyCreated = components['schemas']['APIKeyCreated']

// The document binds `items` to one schema per listing rather than leaving it
// generic, so a page carries the name of what it holds.
export type PagedAccount = components['schemas']['PagedAccount']
export type PagedCertificate = components['schemas']['PagedCertificate']
export type PagedAPIKey = components['schemas']['PagedAPIKey']
export type PagedNotifyChannel = components['schemas']['PagedNotifyChannel']
export type PagedDnsRecord = components['schemas']['PagedDnsRecord']
export type PagedDDNSTask = components['schemas']['PagedDDNSTask']
export type PagedHistoryEntry = components['schemas']['PagedHistoryEntry']
export type PagedIPChange = components['schemas']['PagedIPChange']
