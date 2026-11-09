import { createFileRoute } from '@tanstack/react-router'
import { APIKeys } from '@/features/api-keys'

export const Route = createFileRoute('/_authenticated/api-keys/')({
  component: APIKeys,
})
