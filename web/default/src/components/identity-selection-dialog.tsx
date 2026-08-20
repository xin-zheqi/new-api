import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

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
        <div className='grid gap-2'>
          {identities.map((identity) => {
            return (
              <Button
                key={identity}
                variant='outline'
                className='justify-start'
                disabled={saving}
                onClick={() => chooseIdentity(identity)}
              >
                {t(identityLabels[identity])}
              </Button>
            )
          })}
        </div>
      </DialogContent>
    </Dialog>
  )
}
