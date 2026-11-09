/** TLS certificates issued through ACME DNS-01 against the same accounts. */
import { request } from '@/lib/http'
import { API_PAGE_SIZE, collectPages } from '@/lib/lists'
import type { Certificate, PagedCertificate } from '@/lib/types'

export interface CertificateInput {
  name: string
  /** The DNSPod account whose zones answer the DNS-01 challenge. */
  account_id: number
  domains: string[]
  key_type: string
  output_dir?: string
  auto_renew?: boolean
  notify_on_issue?: boolean
}

/** Every certificate; the table filters client side, so it reads the pages through. */
export function listCertificates(): Promise<Certificate[]> {
  return collectPages((page) =>
    request<PagedCertificate>({
      method: 'get',
      url: '/certificates',
      params: { page, page_size: API_PAGE_SIZE },
    })
  ).then((list) => list.items)
}

export function createCertificate(input: CertificateInput): Promise<Certificate> {
  return request<Certificate>({ method: 'post', url: '/certificates', data: input })
}

export function updateCertificate(
  id: number,
  input: Partial<CertificateInput>
): Promise<Certificate> {
  return request<Certificate>({ method: 'put', url: `/certificates/${id}`, data: input })
}

/** Removes the record of a certificate; the files on disk are left alone. */
export function deleteCertificate(id: number): Promise<null> {
  return request<null>({ method: 'delete', url: `/certificates/${id}` })
}

/**
 * Starts an issuance and returns while it runs: the answer is the certificate
 * with a RUNNING status, and the caller watches the row until it settles.
 */
export function issueCertificate(id: number): Promise<Certificate> {
  return request<Certificate>({ method: 'post', url: `/certificates/${id}/issue` })
}
