import { type ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { CheckCircle2, MoreHorizontal, Pencil, Trash2 } from 'lucide-react'
import { type Account } from '@/lib/types'
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

export type AccountRowActions = {
  onEdit: (account: Account) => void
  onVerify: (account: Account) => void
  onDelete: (account: Account) => void
}

/**
 * The DNSPod credentials this deployment may use.
 *
 * The secret itself is never shown: only the id, because that is what identifies
 * it at DNSPod, and a hint of the key, because matching without any part of it
 * means guessing.
 */
export function useAccountsColumns(actions: AccountRowActions): ColumnDef<Account>[] {
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
      accessorFn: (account) => account.name,
      header: ({ column }) => <DataTableColumnHeader column={column} title={t('common.name')} />,
      cell: ({ row }) => (
        <div className='flex items-center gap-2'>
          <span className='truncate font-medium'>{row.original.name}</span>
          {row.original.is_default ? (
            <Badge variant='secondary'>{t('common.isDefault')}</Badge>
          ) : null}
        </div>
      ),
      enableHiding: false,
    },
    {
      id: 'secret_id',
      accessorFn: (account) => account.secret_id,
      header: ({ column }) => <DataTableColumnHeader column={column} title='SecretId' />,
      cell: ({ row }) => (
        <span className='block truncate font-mono text-xs'>{row.original.secret_id}</span>
      ),
    },
    {
      id: 'key_hint',
      accessorFn: (account) => account.key_hint,
      header: () => <div className='text-sm font-medium'>Key</div>,
      cell: ({ row }) => (
        <span className='text-muted-foreground font-mono text-xs'>
          {row.original.key_hint || '-'}
        </span>
      ),
      enableSorting: false,
    },
    {
      id: 'updated_at',
      accessorFn: (account) => account.updated_at,
      header: ({ column }) => <DataTableColumnHeader column={column} title={t('common.updated')} />,
      cell: ({ row }) => (
        <span className='text-muted-foreground whitespace-nowrap text-sm'>
          {formatTime(row.original.updated_at)}
        </span>
      ),
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
            <DropdownMenuItem onClick={() => actions.onEdit(row.original)}>
              <Pencil />
              {t('common.edit')}
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => actions.onVerify(row.original)}>
              <CheckCircle2 />
              {t('common.verify')}
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
