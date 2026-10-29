import { createFileRoute } from '@tanstack/react-router'
import { Domains } from '@/features/domains'

export const Route = createFileRoute('/_authenticated/domains/')({
  component: Domains,
})
