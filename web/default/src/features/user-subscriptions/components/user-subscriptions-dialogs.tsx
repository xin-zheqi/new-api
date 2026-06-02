/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  sideDrawerContentClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import {
  createUserSubscription,
  getAdminPlans,
  invalidateUserSubscription,
} from '@/features/subscriptions/api'
import type { PlanRecord } from '@/features/subscriptions/types'
import {
  canInvalidateSubscription,
  getSubscriptionSourceLabel,
  getSubscriptionStatusMeta,
} from '../constants'
import type { AdminUserSubscription } from '../types'
import { useUserSubscriptions } from './user-subscriptions-provider'

function DetailRow({
  label,
  value,
}: {
  label: string
  value: ReactNode
}) {
  return (
    <div className='grid grid-cols-[120px_1fr] gap-3 border-b py-2 text-sm last:border-b-0'>
      <div className='text-muted-foreground'>{label}</div>
      <div className='break-all'>{value || '-'}</div>
    </div>
  )
}

function UserSubscriptionDetailsSheet({
  row,
  open,
  onOpenChange,
}: {
  row: AdminUserSubscription | null
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const statusMeta = row
    ? getSubscriptionStatusMeta(row.effective_status, t)
    : null

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-xl')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>{t('View Details')}</SheetTitle>
          <SheetDescription>
            {t('Subscription ID')}: {row?.id || '-'}
          </SheetDescription>
        </SheetHeader>
        {row ? (
          <div className={sideDrawerFormClassName()}>
            <section>
              <h3 className='mb-2 font-medium'>{t('User Info')}</h3>
              <div className='rounded-md border px-3'>
                <DetailRow label='ID' value={row.user_id} />
                <DetailRow
                  label={t('User')}
                  value={row.display_name || row.username || t('Unknown User')}
                />
                <DetailRow label='Email' value={row.email || '-'} />
              </div>
            </section>

            <section>
              <h3 className='mb-2 font-medium'>{t('Plan Info')}</h3>
              <div className='rounded-md border px-3'>
                <DetailRow label='ID' value={row.plan_id} />
                <DetailRow
                  label={t('Plan')}
                  value={row.plan_title || t('Unknown Plan')}
                />
                <DetailRow
                  label={t('Source')}
                  value={getSubscriptionSourceLabel(row.source, t)}
                />
                <DetailRow
                  label={t('Upgrade Group')}
                  value={row.upgrade_group || t('No Upgrade')}
                />
              </div>
            </section>

            <section>
              <h3 className='mb-2 font-medium'>{t('Quota Details')}</h3>
              <div className='rounded-md border px-3'>
                <DetailRow
                  label={t('Total Quota')}
                  value={
                    row.amount_total > 0
                      ? formatQuota(row.amount_total)
                      : t('Unlimited')
                  }
                />
                <DetailRow
                  label={t('Used Quota')}
                  value={formatQuota(row.amount_used)}
                />
                <DetailRow
                  label={t('Remaining Quota')}
                  value={
                    row.amount_total > 0
                      ? formatQuota(row.amount_remaining)
                      : t('Unlimited')
                  }
                />
              </div>
            </section>

            <section>
              <h3 className='mb-2 font-medium'>{t('Period')}</h3>
              <div className='rounded-md border px-3'>
                <DetailRow
                  label={t('Start')}
                  value={formatTimestampToDate(row.start_time)}
                />
                <DetailRow
                  label={t('End')}
                  value={formatTimestampToDate(row.end_time)}
                />
                <DetailRow
                  label={t('Reset Time')}
                  value={formatTimestampToDate(row.next_reset_time)}
                />
                <DetailRow
                  label={t('Created')}
                  value={formatTimestampToDate(row.created_at)}
                />
              </div>
            </section>

            <section>
              <h3 className='mb-2 font-medium'>{t('Status')}</h3>
              <div className='rounded-md border px-3'>
                <DetailRow
                  label={t('Effective Status')}
                  value={statusMeta?.label || '-'}
                />
                <DetailRow label={t('Raw Status')} value={row.status || '-'} />
              </div>
            </section>
          </div>
        ) : null}
      </SheetContent>
    </Sheet>
  )
}

function CreateUserSubscriptionSheet({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const { triggerRefresh } = useUserSubscriptions()
  const [plans, setPlans] = useState<PlanRecord[]>([])
  const [userId, setUserId] = useState('')
  const [planId, setPlanId] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const enabledPlans = useMemo(
    () => plans.filter((item) => item.plan.enabled),
    [plans]
  )

  useEffect(() => {
    if (!open) return
    setUserId('')
    setPlanId('')
    getAdminPlans()
      .then((res) => {
        if (res.success) setPlans(res.data || [])
      })
      .catch(() => toast.error(t('Loading failed')))
  }, [open, t])

  const handleSubmit = async () => {
    const parsedUserId = Number(userId)
    const parsedPlanId = Number(planId)
    if (!Number.isInteger(parsedUserId) || parsedUserId <= 0) {
      toast.error(t('Please enter a valid user ID'))
      return
    }
    if (!Number.isInteger(parsedPlanId) || parsedPlanId <= 0) {
      toast.error(t('Please select a subscription plan'))
      return
    }
    setSubmitting(true)
    try {
      const res = await createUserSubscription(parsedUserId, {
        plan_id: parsedPlanId,
      })
      if (res.success) {
        toast.success(res.data?.message || t('Subscription opened'))
        triggerRefresh()
        onOpenChange(false)
      } else {
        toast.error(res.message || t('Request failed'))
      }
    } catch {
      toast.error(t('Request failed'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-md')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>{t('Open Subscription')}</SheetTitle>
          <SheetDescription>
            {t('Open a subscription for a user')}
          </SheetDescription>
        </SheetHeader>
        <div className={sideDrawerFormClassName()}>
          <div className='space-y-2'>
            <label className='text-sm font-medium'>{t('User ID')}</label>
            <Input
              type='number'
              min={1}
              value={userId}
              placeholder={t('Enter user ID')}
              onChange={(event) => setUserId(event.target.value)}
            />
          </div>
          <div className='space-y-2'>
            <label className='text-sm font-medium'>
              {t('Subscription Plan')}
            </label>
            <Select
              items={enabledPlans.map((item) => ({
                value: String(item.plan.id),
                label: item.plan.title,
              }))}
              value={planId}
              onValueChange={(value) => value !== null && setPlanId(value)}
            >
              <SelectTrigger>
                <SelectValue placeholder={t('Select subscription plan')} />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {enabledPlans.map((item) => (
                    <SelectItem
                      key={item.plan.id}
                      value={String(item.plan.id)}
                    >
                      {item.plan.title} ($
                      {Number(item.plan.price_amount || 0).toFixed(2)})
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>
          <Button onClick={handleSubmit} disabled={submitting}>
            <Plus className='mr-1 h-4 w-4' />
            {submitting ? t('Opening...') : t('Open Subscription')}
          </Button>
          <p className='text-muted-foreground text-xs'>
            {t(
              'This creates a new subscription from the selected plan. Existing subscriptions are not modified.'
            )}
          </p>
        </div>
      </SheetContent>
    </Sheet>
  )
}

export function UserSubscriptionsDialogs() {
  const { t } = useTranslation()
  const { open, setOpen, currentRow, triggerRefresh } = useUserSubscriptions()

  const handleInvalidate = async () => {
    if (!currentRow) return
    if (!canInvalidateSubscription(currentRow.effective_status)) {
      toast.error(t('Cannot invalidate this subscription'))
      setOpen(null)
      return
    }
    try {
      const res = await invalidateUserSubscription(currentRow.id)
      if (res.success) {
        toast.success(res.data?.message || t('Has been invalidated'))
        triggerRefresh()
      } else {
        toast.error(res.message || t('Operation failed'))
      }
    } catch {
      toast.error(t('Operation failed'))
    } finally {
      setOpen(null)
    }
  }

  return (
    <>
      <CreateUserSubscriptionSheet
        open={open === 'create'}
        onOpenChange={(value) => !value && setOpen(null)}
      />
      <UserSubscriptionDetailsSheet
        row={currentRow}
        open={open === 'details'}
        onOpenChange={(value) => !value && setOpen(null)}
      />
      <ConfirmDialog
        open={open === 'invalidate'}
        onOpenChange={(value) => !value && setOpen(null)}
        title={t('Confirm invalidate')}
        desc={t(
          'After invalidating, this subscription will be immediately deactivated. Historical records are not affected. Continue?'
        )}
        handleConfirm={handleInvalidate}
        destructive
      />
    </>
  )
}
