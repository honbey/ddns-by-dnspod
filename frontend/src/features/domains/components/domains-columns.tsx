import { type ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { ChevronRight, Settings2 } from 'lucide-react'
import { type Domain } from '@/lib/types'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DataTableColumnHeader } from '@/components/data-table'

export type DomainRowActions = {
  onImport: (domain: Domain) => void
  /**
   * The account these domains were read from. Every link carries it, so the
   * records page asks the same account instead of the default one - which would
   * answer for a domain it has never heard of.
   */
  accountId: number
}

/**
 * The search a records link carries: which account holds the domain.
 *
 * Zero is the default account, which the records route reads from a missing
 * parameter, so it is left out rather than sent as an id.
 */
function recordSearch(accountId: number): { account_id?: number } {
  return accountId > 0 ? { account_id: accountId } : {}
}

/**
 * The domains the accounts can change.
 *
 * Nothing about records yet: that belongs to the page behind the row, because a
 * table of domains listing a table of records would not survive a narrow window.
 */
export function useDomainsColumns(
  actions: DomainRowActions,
  groupNames: Map<number, string>
): ColumnDef<Domain>[] {
  const { t } = useTranslation()
  return [
    {
      id: 'name',
      accessorFn: (domain) => domain.name,
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('domains.domainColumn')} />
      ),
      cell: ({ row }) => (
        <Button asChild variant='link' className='h-auto p-0 font-medium'>
          <Link
            to='/domains/$domain'
            params={{ domain: row.original.name }}
            search={recordSearch(actions.accountId)}
          >
            {row.original.name}
          </Link>
        </Button>
      ),
      enableHiding: false,
    },
    {
      id: 'records',
      accessorFn: (domain) => domain.records,
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('domains.recordsColumn')} />
      ),
      cell: ({ row }) => <span className='tabular-nums'>{row.original.records}</span>,
      meta: { className: 'w-24' },
    },
    {
      id: 'group',
      // The id is what the faceted filter offers, so the column's own value has
      // to be the id rather than the name shown in the cell.
      accessorFn: (domain) => String(domain.group_id),
      filterFn: (row, id, value: string[]) => value.includes(row.getValue(id)),
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('domains.groupColumn')} />
      ),
      cell: ({ row }) => (
        <span className='text-muted-foreground text-sm'>
          {groupNames.get(row.original.group_id) ?? t('domains.noGroup')}
        </span>
      ),
      meta: { className: 'w-32' },
    },
    {
      id: 'status',
      accessorFn: (domain) => domain.status,
      filterFn: (row, id, value: string[]) => value.includes(row.getValue(id)),
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('domains.statusColumn')} />
      ),
      cell: ({ row }) => (
        <Badge variant={row.original.status === 'enable' ? 'default' : 'secondary'}>
          {row.original.status}
        </Badge>
      ),
      meta: { className: 'w-28' },
    },
    {
      id: 'grade',
      accessorFn: (domain) => domain.grade,
      header: () => <div className='text-sm font-medium'>{t('domains.packageLabel')}</div>,
      cell: ({ row }) => (
        <span className='text-muted-foreground text-sm'>{row.original.grade}</span>
      ),
      enableSorting: false,
      meta: { className: 'w-40' },
    },
    {
      id: 'actions',
      cell: ({ row }) => (
        <div className='flex items-center justify-end gap-1'>
          <Button
            variant='ghost'
            size='icon'
            className='size-8'
            onClick={() => actions.onImport(row.original)}
            aria-label={t('domains.createFromGroup', { name: row.original.name })}
          >
            <Settings2 className='size-4' />
          </Button>
          <Button asChild variant='ghost' size='icon' className='size-8'>
            <Link
              to='/domains/$domain'
              params={{ domain: row.original.name }}
              search={recordSearch(actions.accountId)}
              aria-label={t('domains.open', { name: row.original.name })}
            >
              <ChevronRight className='size-4' />
            </Link>
          </Button>
        </div>
      ),
      enableSorting: false,
      enableHiding: false,
      meta: { className: 'w-20' },
    },
  ]
}
