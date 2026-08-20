import { createFileRoute } from '@tanstack/react-router'
import { IdentityReviews } from '@/features/identity-reviews'

export const Route = createFileRoute('/_authenticated/identity-reviews')({ component: IdentityReviews })
