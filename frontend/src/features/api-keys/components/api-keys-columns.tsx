import { type ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { API_KEY_ANY_DOMAIN } from '@/features/api-keys/api'
import { MoreHorizontal, Pencil, Trash2 } from 'lucide-react'
import { type APIKey } from '@/lib/types'
import { formatTime } from '@/lib/format'
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
import { RowSwitch } from '@/components/data-table/row-switch'

export type APIKeyRowActions = {
  onEdit: (key: APIKey) => void
  /** Resolves when the change has landed, so the switch knows it can let go. */
  onToggle: (key: APIKey) => Promise<unknown>
  onDelete: (key: APIKey) => void
}

/**
 * What a key may reach: one domain, or every one of its account. The wildcard
 * is stored as the character itself, which reads badly in a table.
 */
function keyScope(key: APIKey): string {
  return key.domain === API_KEY_ANY_DOMAIN ? 'every domain' : key.domain
}

/**
 * Credentials for scripts.
 *
 * Only the prefix is stored, so nothing here can hand a secret back: what is
 * shown is enough to recognise which key this is and nothing else.
 */
export function useApiKeysColumns(
  actions: APIKeyRowActions,
  accountNames: Map<number, string>
): ColumnDef<APIKey>[] {
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
      accessorFn: (key) => key.name,
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('apiKeys.nameColumn')} />
      ),
      cell: ({ row }) => (
        <div className='truncate font-medium' title={`${row.original.key_prefix}...`}>
          {row.original.name}
        </div>
      ),
      enableHiding: false,
    },
    {
      id: 'domain',
      accessorFn: (key) => key.domain,
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('apiKeys.scopeColumn')} />
      ),
      cell: ({ row }) => <span className='truncate text-sm'>{keyScope(row.original)}</span>,
    },
    {
      id: 'account_id',
      accessorFn: (key) => accountNames.get(key.account_id) ?? `#${key.account_id}`,
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('apiKeys.accountColumn')} />
      ),
      cell: ({ row }) => (
        <span className='text-muted-foreground truncate text-sm'>
          {accountNames.get(row.original.account_id) ?? `#${row.original.account_id}`}
        </span>
      ),
      meta: { className: 'w-32' },
    },
    {
      id: 'last_used_at',
      accessorFn: (key) => key.last_used_at ?? '',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('apiKeys.lastUsedColumn')} />
      ),
      cell: ({ row }) => (
        <span className='text-muted-foreground whitespace-nowrap text-sm'>
          {row.original.last_used_at ? formatTime(row.original.last_used_at) : 'never'}
        </span>
      ),
    },
    {
      id: 'enabled',
      accessorFn: (key) => (key.enabled ? 'on' : 'off'),
      filterFn: (row, id, value: string[]) => value.includes(row.getValue(id)),
      header: () => <div className='text-sm font-medium'>{t('status.enabled')}</div>,
      cell: ({ row }) => (
        <RowSwitch
          checked={row.original.enabled}
          onToggle={() => actions.onToggle(row.original)}
          label={`${row.original.enabled ? t('records.disable') : t('records.enable')} ${row.original.name}`}
        />
      ),
      enableSorting: false,
      meta: { className: 'w-20' },
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
