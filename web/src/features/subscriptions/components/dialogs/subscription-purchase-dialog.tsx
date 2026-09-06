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
import { Crown, CalendarClock, Package } from 'lucide-react'
import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { GroupBadge } from '@/components/group-badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { WechatPayDialog } from '@/features/wallet/components/dialogs/wechat-pay-dialog'
import {
  getPaymentMethodDisplayName,
  isPaymentMethodEnabled,
} from '@/features/wallet/lib/payment-methods'
import type { PaymentMethodSetting } from '@/features/wallet/types'
import { formatQuota } from '@/lib/format'
import {
  openHttpCheckoutUrl,
  redirectToHttpCheckoutUrl,
  submitHttpCheckoutForm,
} from '@/lib/payment-redirect'

import {
  paySubscriptionStripe,
  paySubscriptionCreem,
  paySubscriptionEpay,
  paySubscriptionWaffoPancake,
  paySubscriptionWechat,
} from '../../api'
import { formatDuration, formatResetPeriod } from '../../lib'
import type { PlanRecord } from '../../types'

interface PaymentMethod {
  type: string
  name?: string
}

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  plan: PlanRecord | null
  enableStripe?: boolean
  enableCreem?: boolean
  enableWaffoPancake?: boolean
  enableOnlineTopUp?: boolean
  enableWechat?: boolean
  epayMethods?: PaymentMethod[]
  paymentMethodSettings?: PaymentMethodSetting[]
  purchaseLimit?: number
  purchaseCount?: number
  onPurchaseSuccess?: () => void | Promise<void>
}

export function SubscriptionPurchaseDialog(props: Props) {
  const { t } = useTranslation()
  const [paying, setPaying] = useState(false)
  const [selectedEpayMethod, setSelectedEpayMethod] = useState('')
  const [wechatDialogOpen, setWechatDialogOpen] = useState(false)
  const [wechatPayment, setWechatPayment] = useState<{
    trade_no: string
    code_url: string
    amount?: string
    currency?: string
    expires_at?: number
  } | null>(null)

  useEffect(() => {
    const availableEpayMethods = (props.epayMethods || []).filter(
      (method) =>
        !method.type.startsWith('waffo:') &&
        isPaymentMethodEnabled(method.type, props.paymentMethodSettings)
    )
    if (props.open && availableEpayMethods.length > 0) {
      setSelectedEpayMethod(availableEpayMethods[0].type)
    } else if (!props.open) {
      setSelectedEpayMethod('')
    }
  }, [props.open, props.epayMethods, props.paymentMethodSettings])

  const plan = props.plan?.plan
  if (!plan) return null

  const hasStripe =
    props.enableStripe &&
    isPaymentMethodEnabled('stripe', props.paymentMethodSettings) &&
    !!plan.stripe_price_id
  const hasCreem =
    props.enableCreem &&
    isPaymentMethodEnabled('creem', props.paymentMethodSettings) &&
    !!plan.creem_product_id
  const hasWaffoPancake =
    props.enableWaffoPancake &&
    isPaymentMethodEnabled('waffo_pancake', props.paymentMethodSettings) &&
    !!plan.waffo_pancake_product_id
  const availableEpayMethods = (props.epayMethods || []).filter(
    (method) =>
      !method.type.startsWith('waffo:') &&
      isPaymentMethodEnabled(method.type, props.paymentMethodSettings)
  )
  const hasEpay = props.enableOnlineTopUp && availableEpayMethods.length > 0
  const hasWechat =
    props.enableWechat === true &&
    isPaymentMethodEnabled('wechat_native_v3', props.paymentMethodSettings)
  const hasAnyPayment =
    hasStripe || hasCreem || hasWaffoPancake || hasEpay || hasWechat
  const selectedEpayMethodLabel =
    (() => {
      const method = availableEpayMethods.find(
        (item) => item.type === selectedEpayMethod
      )
      return method
        ? getPaymentMethodDisplayName(
            method.type,
            method.name,
            props.paymentMethodSettings,
            t
          )
        : undefined
    })() ||
    selectedEpayMethod ||
    t('Select payment method')
  const stripeName = getPaymentMethodDisplayName(
    'stripe',
    'Stripe',
    props.paymentMethodSettings,
    t
  )
  const creemName = getPaymentMethodDisplayName(
    'creem',
    'Creem',
    props.paymentMethodSettings,
    t
  )
  const pancakeName = getPaymentMethodDisplayName(
    'waffo_pancake',
    'Waffo Pancake',
    props.paymentMethodSettings,
    t
  )
  const wechatName = getPaymentMethodDisplayName(
    'wechat_native_v3',
    'WeChat Pay',
    props.paymentMethodSettings,
    t
  )
  const totalAmount = Number(plan.total_amount || 0)
  const price = Number(plan.price_amount || 0).toFixed(2)
  const limitReached =
    (props.purchaseLimit || 0) > 0 &&
    (props.purchaseCount || 0) >= (props.purchaseLimit || 0)

  const handlePayStripe = async () => {
    setPaying(true)
    try {
      const res = await paySubscriptionStripe({ plan_id: plan.id })
      if (res.message === 'success' && res.data?.pay_link) {
        if (!openHttpCheckoutUrl(res.data.pay_link)) {
          toast.error(t('Invalid payment redirect URL'))
          return
        }
        toast.success(t('Payment page opened'))
        props.onOpenChange(false)
      } else {
        toast.error(
          res.message && res.message !== 'success'
            ? res.message
            : t('Payment request failed')
        )
      }
    } catch {
      toast.error(t('Payment request failed'))
    } finally {
      setPaying(false)
    }
  }

  const handlePayCreem = async () => {
    setPaying(true)
    try {
      const res = await paySubscriptionCreem({ plan_id: plan.id })
      if (res.message === 'success' && res.data?.checkout_url) {
        if (!openHttpCheckoutUrl(res.data.checkout_url)) {
          toast.error(t('Invalid payment redirect URL'))
          return
        }
        toast.success(t('Payment page opened'))
        props.onOpenChange(false)
      } else {
        toast.error(
          res.message && res.message !== 'success'
            ? res.message
            : t('Payment request failed')
        )
      }
    } catch {
      toast.error(t('Payment request failed'))
    } finally {
      setPaying(false)
    }
  }

  // In-tab redirect (not window.open) — user-gesture context is lost
  // across the await, so a popup would be blocked. Same as the wallet hook.
  const handlePayWaffoPancake = async () => {
    setPaying(true)
    try {
      const res = await paySubscriptionWaffoPancake({ plan_id: plan.id })
      if (res.message === 'success' && res.data?.checkout_url) {
        if (!redirectToHttpCheckoutUrl(res.data.checkout_url)) {
          toast.error(t('Invalid payment redirect URL'))
          return
        }
        toast.success(t('Redirecting to payment page...'))
      } else {
        toast.error(
          res.message && res.message !== 'success'
            ? res.message
            : t('Payment request failed')
        )
      }
    } catch {
      toast.error(t('Payment request failed'))
    } finally {
      setPaying(false)
    }
  }

  const handlePayEpay = async () => {
    if (!selectedEpayMethod) {
      toast.error(t('Please select a payment method'))
      return
    }
    setPaying(true)
    try {
      const res = await paySubscriptionEpay({
        plan_id: plan.id,
        payment_method: selectedEpayMethod,
      })
      if (res.message === 'success' && res.url) {
        if (!submitHttpCheckoutForm(res.url, res.data || {})) {
          toast.error(t('Invalid payment redirect URL'))
          return
        }
        toast.success(t('Payment initiated'))
        props.onOpenChange(false)
      } else {
        toast.error(
          res.message && res.message !== 'success'
            ? res.message
            : t('Payment request failed')
        )
      }
    } catch {
      toast.error(t('Payment request failed'))
    } finally {
      setPaying(false)
    }
  }

  const handlePayWechat = async () => {
    setPaying(true)
    try {
      const res = await paySubscriptionWechat({ plan_id: plan.id })
      if (
        (res.success || res.message === 'success') &&
        res.data?.code_url &&
        res.data.trade_no
      ) {
        setWechatPayment({
          trade_no: res.data.trade_no,
          code_url: res.data.code_url,
          amount: res.data.amount,
          currency: res.data.currency,
          expires_at:
            typeof res.data.expires_at === 'string'
              ? Number(res.data.expires_at)
              : res.data.expires_at,
        })
        props.onOpenChange(false)
        setWechatDialogOpen(true)
      } else {
        toast.error(res.message || t('Payment request failed'))
      }
    } catch {
      toast.error(t('Payment request failed'))
    } finally {
      setPaying(false)
    }
  }

  const purchaseDialog = (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={
        <>
          <Crown className='h-5 w-5' />
          {t('Purchase Subscription')}
        </>
      }
      contentClassName='max-sm:w-[calc(100vw-1.5rem)] sm:max-w-md'
      titleClassName='flex items-center gap-2'
      contentHeight='auto'
      bodyClassName='space-y-4'
    >
      <div className='space-y-3 sm:space-y-4'>
        <div className='bg-muted/50 space-y-2.5 rounded-lg border p-3 sm:space-y-3 sm:p-4'>
          <div className='flex justify-between'>
            <span className='text-muted-foreground text-sm'>
              {t('Plan Name')}
            </span>
            <span className='max-w-[200px] truncate text-sm font-medium'>
              {plan.title}
            </span>
          </div>
          <div className='flex items-center justify-between'>
            <span className='text-muted-foreground text-sm'>
              {t('Validity Period')}
            </span>
            <span className='flex items-center gap-1 text-sm'>
              <CalendarClock className='h-3.5 w-3.5' />
              {formatDuration(plan, t)}
            </span>
          </div>
          {formatResetPeriod(plan, t) !== t('No Reset') && (
            <div className='flex justify-between'>
              <span className='text-muted-foreground text-sm'>
                {t('Reset Period')}
              </span>
              <span className='text-sm'>{formatResetPeriod(plan, t)}</span>
            </div>
          )}
          <div className='flex items-center justify-between'>
            <span className='text-muted-foreground text-sm'>
              {t('Plan Quota')}
            </span>
            <span className='flex items-center gap-1 text-sm'>
              <Package className='h-3.5 w-3.5' />
              {totalAmount > 0 ? formatQuota(totalAmount) : t('Unlimited')}
            </span>
          </div>
          {plan.upgrade_group && (
            <div className='flex items-center justify-between'>
              <span className='text-muted-foreground text-sm'>
                {t('Upgrade Group')}
              </span>
              <GroupBadge group={plan.upgrade_group} />
            </div>
          )}
          <Separator />
          <div className='flex items-center justify-between'>
            <span className='text-sm font-medium'>{t('Amount Due')}</span>
            <span className='text-primary text-lg font-bold'>${price}</span>
          </div>
        </div>

        {limitReached && (
          <Alert variant='destructive'>
            <AlertDescription>
              {t('Purchase limit reached')} ({props.purchaseCount}/
              {props.purchaseLimit})
            </AlertDescription>
          </Alert>
        )}

        {hasAnyPayment && (
          <div className='space-y-3'>
            <p className='text-muted-foreground text-xs'>
              {t('Select payment method')}
            </p>
            {(hasStripe || hasCreem || hasWaffoPancake) && (
              <div className='grid grid-cols-2 gap-2 sm:flex'>
                {hasStripe && (
                  <Button
                    variant='outline'
                    className='flex-1'
                    onClick={handlePayStripe}
                    disabled={paying || limitReached}
                  >
                    {stripeName}
                  </Button>
                )}
                {hasCreem && (
                  <Button
                    variant='outline'
                    className='flex-1'
                    onClick={handlePayCreem}
                    disabled={paying || limitReached}
                  >
                    {creemName}
                  </Button>
                )}
                {hasWaffoPancake && (
                  <Button
                    variant='outline'
                    className='flex-1'
                    onClick={handlePayWaffoPancake}
                    disabled={paying || limitReached}
                  >
                    {pancakeName}
                  </Button>
                )}
              </div>
            )}
            {hasEpay && (
              <div className='grid grid-cols-[minmax(0,1fr)_auto] gap-2'>
                <Select
                  items={availableEpayMethods.map((m) => ({
                    value: m.type,
                    label: getPaymentMethodDisplayName(
                      m.type,
                      m.name,
                      props.paymentMethodSettings,
                      t
                    ),
                  }))}
                  value={selectedEpayMethod}
                  onValueChange={(v) => v !== null && setSelectedEpayMethod(v)}
                  disabled={limitReached}
                >
                  <SelectTrigger className='flex-1'>
                    <SelectValue>{selectedEpayMethodLabel}</SelectValue>
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      {availableEpayMethods.map((m) => (
                        <SelectItem key={m.type} value={m.type}>
                          {getPaymentMethodDisplayName(
                            m.type,
                            m.name,
                            props.paymentMethodSettings,
                            t
                          )}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <Button
                  onClick={handlePayEpay}
                  disabled={paying || !selectedEpayMethod || limitReached}
                >
                  {t('Pay')}
                </Button>
              </div>
            )}
            {hasWechat && (
              <Button
                variant='outline'
                className='w-full'
                onClick={handlePayWechat}
                disabled={paying || limitReached}
              >
                {wechatName}
              </Button>
            )}
          </div>
        )}
      </div>
    </Dialog>
  )

  return (
    <>
      {purchaseDialog}
      {wechatPayment && (
        <WechatPayDialog
          open={wechatDialogOpen}
          onOpenChange={(open) => {
            setWechatDialogOpen(open)
            if (!open) setWechatPayment(null)
          }}
          tradeNo={wechatPayment.trade_no}
          codeUrl={wechatPayment.code_url}
          amount={wechatPayment.amount}
          currency={wechatPayment.currency}
          expiresAt={wechatPayment.expires_at}
          onSuccess={props.onPurchaseSuccess}
        />
      )}
    </>
  )
}
