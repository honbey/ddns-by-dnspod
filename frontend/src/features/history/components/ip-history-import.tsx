import { useRef } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { type TFunction } from 'i18next'
import { Loader2, Upload } from 'lucide-react'
import { toast } from 'sonner'
import { importIPHistory } from '@/features/history/api'
import { handleServerError } from '@/lib/handle-server-error'
import { type IPHistoryImportResult } from '@/lib/types'
import { Button } from '@/components/ui/button'

type ImportIssue = IPHistoryImportResult['errors'][number]

/**
 * The server sends a stable code plus English wording for its logs; the code
 * is what the interface translates, so a Chinese page does not end a sentence
 * in English. An unknown code - a newer server - falls back to the wording
 * rather than showing nothing.
 */
function describeIssue(t: TFunction, issue: ImportIssue): string {
  switch (issue.code) {
    case 'time_invalid':
      return t('history.ipErrorTimeInvalid')
    case 'time_future':
      return t('history.ipErrorTimeFuture')
    case 'time_too_old':
      return t('history.ipErrorTimeTooOld')
    case 'ip_invalid':
      return t('history.ipErrorIpInvalid')
    case 'family_invalid':
      return t('history.ipErrorFamilyInvalid')
    case 'family_mismatch':
      return t('history.ipErrorFamilyMismatch')
    case 'row_invalid':
      return t('history.ipErrorRowInvalid')
    case 'duplicate_row':
      return t('history.ipErrorDuplicateRow')
    default:
      return issue.message
  }
}

/**
 * Merges a CSV of `time,ip[,family]` into the address timeline.
 *
 * The file picker is an input rather than a button because the browser owns
 * the dialog; the button only clicks it. Resetting the value before sending
 * keeps a second pick of the same file firing a change event - otherwise a
 * failed import could not be retried with the corrected file of the same name.
 */
export function IPHistoryImport() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const fileInput = useRef<HTMLInputElement>(null)

  const mutation = useMutation({
    mutationFn: importIPHistory,
    onSuccess: (result) => {
      const shown = result.errors.slice(0, 3)
      // skipped counts every row not stored, while errors stops at the
      // server's cap; the remainder has to come from skipped or a truncated
      // error list would understate it.
      const remaining = result.skipped - shown.length
      const detail = result.errors.length
        ? [
            ...shown.map(
              (entry) =>
                `${t('history.ipImportLine', { line: entry.line })}: ${describeIssue(t, entry)}`
            ),
            ...(remaining > 0 ? [t('history.ipImportMore', { count: remaining })] : []),
          ].join('; ')
        : undefined

      toast.success(
        t('history.ipImportDone', { imported: result.imported, skipped: result.skipped }),
        {
          description: detail,
        }
      )
      void queryClient.invalidateQueries({ queryKey: ['ip-changes'] })
    },
    onError: (error) => handleServerError(error, t('history.ipImportFailed')),
  })

  return (
    <>
      <input
        ref={fileInput}
        type='file'
        accept='.csv,text/csv'
        className='hidden'
        onChange={(event) => {
          const file = event.target.files?.[0]
          event.target.value = ''
          if (file) mutation.mutate(file)
        }}
      />
      <Button
        variant='outline'
        size='sm'
        className='h-8'
        disabled={mutation.isPending}
        onClick={() => fileInput.current?.click()}
      >
        {mutation.isPending ? <Loader2 className='animate-spin' /> : <Upload />}
        {t('history.ipImport')}
      </Button>
    </>
  )
}
