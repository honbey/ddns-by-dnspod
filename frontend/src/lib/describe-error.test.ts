import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { i18n, i18nReady } from '@/i18n'
import { ApiError } from '@/lib/http'
import { describeError } from './describe-error'

const SERVER_WORDING = 'a task for this host already exists'

describe('describeError', () => {
  let originalLanguage: string

  beforeAll(async () => {
    await i18nReady
    originalLanguage = i18n.language
    await i18n.changeLanguage('zh')
  })

  afterAll(async () => {
    await i18n.changeLanguage(originalLanguage)
  })

  it('translates a duplicate task by its code, not the server wording', () => {
    const shown = describeError(new ApiError(SERVER_WORDING, 409, { code: 'duplicate_task' }))
    expect(shown).toBe(i18n.t('common.duplicateTask'))
    expect(shown).not.toBe(SERVER_WORDING)
  })

  it('keeps the server wording for a conflict with no known code', () => {
    expect(describeError(new ApiError('an account with this name already exists', 409))).toBe(
      'an account with this name already exists'
    )
  })

  it('still names the request id after translating', () => {
    const shown = describeError(
      new ApiError('x', 409, { code: 'duplicate_task', requestId: 'rid-1' })
    )
    expect(shown).toBe(
      i18n.t('common.withRequestId', {
        message: i18n.t('common.duplicateTask'),
        requestId: 'rid-1',
      })
    )
  })
})
