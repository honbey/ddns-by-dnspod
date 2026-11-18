import { type ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { MoreHorizontal, Pencil, Play, Trash2 } from 'lucide-react'
import { describeCron } from '@/lib/cron'
import { type Certificate } from '@/lib/types'
import { formatTime } from '@/lib/format'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { DataTableColumnHeader } from '@/components/data-table'
import { LastRunCell } from '@/components/data-table/last-run-cell'
import { RowSwitch } from '@/components/data-table/row-switch'
import { expiryLabel, expiryOf, keyTypeLabel } from './certificate-types'

export type CertificateRowActions = {
  /** Starts an issuance; from there the row reports the run. */
  onIssue: (cert: Certificate) => void
  onEdit: (cert: Certificate) => void
  /** Resolves when the change has landed, so the switch knows it can let go. */
  onToggleNotify: (cert: Certificate) => Promise<unknown>
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
      id: 'select',
      header: ({ table }) => (
        <Checkbox
          checked={
            table.getIsAllPageRowsSelected() ||
            (table.getIsSomePageRowsSelected() && 'indeterminate')
          }
          onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
          aria-label={t('common.selectAll')}
          className='translate-y-[2px]'
        />
      ),
      cell: ({ row }) => (
        <Checkbox
          checked={row.getIsSelected()}
          onCheckedChange={(value) => row.toggleSelected(!!value)}
          aria-label={t('common.selectRow')}
          className='translate-y-[2px]'
        />
      ),
      enableSorting: false,
      enableHiding: false,
      meta: { className: 'w-8' },
    },
    {
      id: 'name',
      accessorFn: (cert) => cert.name,
      header: ({ column }) => <DataTableColumnHeader column={column} title={t('common.name')} />,
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
      header: ({ column }) => <DataTableColumnHeader column={column} title={t('ui.account')} />,
      cell: ({ row }) => (
        <span className='text-muted-foreground block truncate text-sm'>
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
            <Badge variant={expiry.variant}>{expiryLabel(expiry, t)}</Badge>
            <div className='text-muted-foreground text-xs'>
              {row.original.not_after ? formatTime(row.original.not_after) : '-'}
            </div>
          </div>
        )
      },
      meta: { className: 'w-40' },
    },
    {
      id: 'renew_cron',
      header: () => <div className='text-sm font-medium'>{t('certificates.renewCronLabel')}</div>,
      cell: ({ row }) => (
        <span className='text-muted-foreground block truncate text-sm'>
          {row.original.renew_cron
            ? describeCron(row.original.renew_cron, t)
            : t('certificates.renewManual')}
        </span>
      ),
      enableSorting: false,
      meta: { className: 'w-32' },
    },
    {
      id: 'last_status',
      accessorFn: (cert) => cert.last_status,
      filterFn: (row, id, value: string[]) => value.includes(row.getValue(id)),
      header: ({ column }) => <DataTableColumnHeader column={column} title={t('ui.lastRun')} />,
      cell: ({ row }) => (
        <LastRunCell
          at={row.original.last_issued_at}
          status={row.original.last_status}
          message={row.original.last_message}
          failCount={row.original.fail_count}
        />
      ),
      meta: { className: 'w-56' },
    },
    {
      id: 'notify_on_issue',
      header: () => <div className='text-sm font-medium'>{t('certificates.notifyLabel')}</div>,
      cell: ({ row }) => (
        <RowSwitch
          checked={row.original.notify_on_issue}
          onToggle={() => actions.onToggleNotify(row.original)}
          label={`${t(row.original.notify_on_issue ? 'ui.disable' : 'ui.enable')} ${
            row.original.name
          }`}
        />
      ),
      enableSorting: false,
      meta: { className: 'w-20' },
    },
    {
      id: 'key_type',
      accessorFn: (cert) => cert.key_type,
      header: () => <div className='text-sm font-medium'>{t('certificates.keyTypeLabel')}</div>,
      cell: ({ row }) => (
        <span className='text-muted-foreground text-sm'>{keyTypeLabel(row.original.key_type)}</span>
      ),
      enableSorting: false,
      meta: { className: 'w-32' },
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
              {row.original.not_after ? t('certificates.renewNow') : t('certificates.requestNow')}
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
