import { type ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { Loader2, MoreHorizontal, Pencil, Play, Trash2 } from 'lucide-react'
import { type Certificate } from '@/lib/types'
import { formatTime, statusVariant } from '@/lib/format'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Paste } from '@/components/paste'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { DataTableColumnHeader } from '@/components/data-table'
import { expiryOf, keyTypeLabel } from './certificate-types'

export type CertificateRowActions = {
  /** Starts an issuance; from there the row reports the run. */
  onIssue: (cert: Certificate) => void
  onEdit: (cert: Certificate) => void
  onDelete: (cert: Certificate) => void
}

/**
 * The certificates, as one table.
 *
 * Every column answers a question someone asks before touching this feature:
 * which names it covers, which account answers for them, how long is left, and
 * where the files are - that last one being the thing the TLS application on
 * the other side is configured with.
 */
export function useCertificatesColumns(
  actions: CertificateRowActions,
  accountNames: Map<number, string>,
  /** False while this deployment has issuance switched off. */
  canIssue: boolean
): ColumnDef<Certificate>[] {
  const { t } = useTranslation()
  return [
    {
      id: 'name',
      accessorFn: (cert) => cert.name,
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('certificates.nameColumn')} />
      ),
      cell: ({ row }) => (
        <div className='min-w-0 space-y-0.5'>
          <div className='truncate font-medium'>{row.original.name}</div>
          <div
            className='text-muted-foreground truncate text-xs'
            title={row.original.domains.join(', ')}
          >
            {row.original.domains.join(', ')}
          </div>
        </div>
      ),
      enableHiding: false,
    },
    {
      id: 'account_id',
      accessorFn: (cert) => accountNames.get(cert.account_id) ?? `#${cert.account_id}`,
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('certificates.accountColumn')} />
      ),
      cell: ({ row }) => (
        <span className='text-muted-foreground truncate text-sm'>
          {accountNames.get(row.original.account_id) ?? `#${row.original.account_id}`}
        </span>
      ),
      meta: { className: 'w-32' },
    },
    {
      id: 'expires',
      accessorFn: (cert) => cert.not_after ?? '',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('certificates.expiresColumn')} />
      ),
      cell: ({ row }) => {
        const expiry = expiryOf(row.original)
        return (
          <div className='space-y-0.5'>
            <Badge variant={expiry.variant}>{expiry.label}</Badge>
            <div className='text-muted-foreground text-xs'>
              {row.original.not_after ? formatTime(row.original.not_after) : '-'}
            </div>
          </div>
        )
      },
      meta: { className: 'w-40' },
    },
    {
      id: 'auto_renew',
      accessorFn: (cert) => (cert.auto_renew ? 'automatic' : 'manual'),
      filterFn: (row, id, value: string[]) => value.includes(row.getValue(id)),
      header: () => <div className='text-sm font-medium'>{t('certificates.renewLabel')}</div>,
      cell: ({ row }) => (
        <span className='text-muted-foreground text-sm'>
          {row.original.auto_renew ? 'automatic' : 'manual'}
        </span>
      ),
      enableSorting: false,
      meta: { className: 'w-28' },
    },
    {
      id: 'last_status',
      accessorFn: (cert) => cert.last_status,
      filterFn: (row, id, value: string[]) => value.includes(row.getValue(id)),
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('certificates.lastRunColumn')} />
      ),
      cell: ({ row }) => (
        // The message is the only place a failure explains itself in the list,
        // so it sits under the badge and is repeated in full on hover.
        <div className='space-y-0.5' title={row.original.last_message}>
          <Badge variant={statusVariant(row.original.last_status)}>
            {row.original.last_status === 'RUNNING' ? (
              <Loader2 aria-hidden='true' className='size-3 animate-spin' />
            ) : null}
            {row.original.last_status || 'never run'}
          </Badge>
          {row.original.fail_count > 0 ? (
            <div className='text-muted-foreground text-xs'>
              {row.original.fail_count} failed in a row
            </div>
          ) : null}
          <div className='text-muted-foreground max-w-64 truncate text-xs'>
            {row.original.last_message}
          </div>
        </div>
      ),
      meta: { className: 'w-56' },
    },
    {
      id: 'key_type',
      accessorFn: (cert) => cert.key_type,
      header: () => <div className='text-sm font-medium'>Key</div>,
      cell: ({ row }) => (
        <span className='text-muted-foreground text-sm'>{keyTypeLabel(row.original.key_type)}</span>
      ),
      enableSorting: false,
      meta: { className: 'w-32' },
    },
    {
      id: 'output_path',
      accessorFn: (cert) => cert.output_path,
      header: () => <div className='text-sm font-medium'>{t('certificates.filesLabel')}</div>,
      cell: ({ row }) => (
        <Paste value={row.original.output_path} label={t('certificates.directoryAria')} />
      ),
      enableSorting: false,
      meta: { className: 'w-64' },
    },
    {
      id: 'actions',
      cell: ({ row }) => (
        <DropdownMenu modal={false}>
          <DropdownMenuTrigger asChild>
            <Button variant='ghost' size='icon' className='size-8'>
              <MoreHorizontal className='size-4' />
              <span className='sr-only'>{t('common.openMenu')}</span>
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align='end' className='w-44'>
            <DropdownMenuItem
              onClick={() => actions.onIssue(row.original)}
              // A disabled item takes no hover, so the reason it is off is
              // spelled out on the page instead of in a title here.
              disabled={!canIssue || row.original.last_status === 'RUNNING'}
            >
              <Play />
              {row.original.not_after ? t('certificates.renewNow') : t('certificates.issueNow')}
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => actions.onEdit(row.original)}>
              <Pencil />
              {t('common.edit')}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem variant='destructive' onClick={() => actions.onDelete(row.original)}>
              <Trash2 />
              {t('common.delete')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      ),
      enableSorting: false,
      enableHiding: false,
      meta: { className: 'w-10' },
    },
  ]
}
