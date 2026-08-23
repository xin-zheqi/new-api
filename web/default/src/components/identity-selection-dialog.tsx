import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Building2, GraduationCap, UserRound, UsersRound } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

const identities = ['personal', 'student', 'university', 'enterprise'] as const
const identityLabels = {
  personal: 'Personal',
  student: 'Student',
  university: 'University',
  enterprise: 'Enterprise',
} as const
const identityIcons = { personal: UserRound, student: GraduationCap, university: Building2, enterprise: UsersRound } as const

export function IdentitySelectionDialog() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const setUser = useAuthStore((state) => state.auth.setUser)
  const [saving, setSaving] = useState(false)
  const hasPendingIdentityReview =
    user?.identity_review_status === 'pending' ||
    user?.identity_requested === 'university' ||
    user?.identity_requested === 'enterprise'
  const shouldSelectIdentity = Boolean(user && !user.identity && !hasPendingIdentityReview)

  const chooseIdentity = async (identity: (typeof identities)[number]) => {
    if (!user) return
    setSaving(true)
    try {
      const response = await api.put('/api/user/self', { identity })
      if (!response.data?.success) {
        toast.error(response.data?.message || t('Failed to update identity'))
        return
      }
      setUser(identity === 'university' || identity === 'enterprise'
        ? { ...user, identity: 'personal', identity_requested: identity, identity_review_status: 'pending' }
        : { ...user, identity })
    } catch {
      toast.error(t('Failed to update identity'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={shouldSelectIdentity} onOpenChange={() => undefined}>
      <DialogContent
        className='sm:max-w-md'
        showCloseButton={false}
      >
        <DialogHeader>
          <DialogTitle>{t('Choose your identity')}</DialogTitle>
          <DialogDescription>
            {t('Select an identity to finish setting up your account.')}
          </DialogDescription>
        </DialogHeader>
        <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
          {identities.map((identity) => {
            const Icon = identityIcons[identity]
            return (
              <Button
                key={identity}
                variant='outline'
                className='h-auto min-h-20 justify-start gap-3 px-4 py-4 text-left'
                disabled={saving}
                onClick={() => chooseIdentity(identity)}
              >
                <Icon className='size-5 shrink-0 text-primary' />
                <span className='font-medium'>{t(identityLabels[identity])}</span>
              </Button>
            )
          })}
        </div>
      </DialogContent>
    </Dialog>
  )
}
