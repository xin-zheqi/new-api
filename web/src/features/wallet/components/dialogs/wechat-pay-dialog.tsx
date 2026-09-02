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
import {
  CheckmarkCircle02Icon,
  InformationCircleIcon,
  MultiplicationSignCircleIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { QRCodeSVG } from 'qrcode.react'
import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { CopyButton } from '@/components/copy-button'
import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'

import { queryWechatOrder } from '../../api'

type PaymentState = 'pending' | 'success' | 'failed' | 'expired' | 'error'

function isTerminalPaymentState(state: PaymentState) {
  return state === 'success' || state === 'failed' || state === 'expired'
}

interface WechatPayDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  tradeNo: string
  codeUrl: string
  amount?: string
  currency?: string
  expiresAt?: number
  onSuccess?: () => void | Promise<void>
}

export function WechatPayDialog(props: WechatPayDialogProps) {
  const { t } = useTranslation()
  const [state, setState] = useState<PaymentState>('pending')
  const [checking, setChecking] = useState(false)
  const [remaining, setRemaining] = useState('')
  const stateRef = useRef<PaymentState>('pending')
  const checkingRef = useRef(false)
  const successHandledRef = useRef(false)
  const onSuccessRef = useRef(props.onSuccess)

  useEffect(() => {
    onSuccessRef.current = props.onSuccess
  }, [props.onSuccess])

  const updateState = useCallback((next: PaymentState) => {
    stateRef.current = next
    setState(next)
  }, [])

  const checkOrder = useCallback(async () => {
    if (
      !props.tradeNo ||
      checkingRef.current ||
      isTerminalPaymentState(stateRef.current)
    ) {
      return
    }

    checkingRef.current = true
    setChecking(true)
    try {
      const response = await queryWechatOrder(props.tradeNo)
      if (!response.success || !response.data) {
        updateState('error')
        return
      }
      switch (response.data.status) {
        case 'success':
          updateState('success')
          if (!successHandledRef.current) {
            successHandledRef.current = true
            toast.success(t('Payment completed'))
            await onSuccessRef.current?.()
          }
          return
        case 'failed':
          updateState('failed')
          return
        case 'expired':
          updateState('expired')
          return
        case 'pending':
          updateState('pending')
          return
        default:
          updateState('error')
      }
    } catch {
      updateState('error')
    } finally {
      checkingRef.current = false
      setChecking(false)
    }
  }, [props.tradeNo, t, updateState])

  useEffect(() => {
    if (!props.open || !props.tradeNo) return

    updateState('pending')
    successHandledRef.current = false
    let cancelled = false
    let timer: number | undefined

    const poll = async () => {
      if (cancelled) return
      await checkOrder()
      if (!cancelled && !isTerminalPaymentState(stateRef.current)) {
        timer = window.setTimeout(poll, 3000)
      }
    }

    void poll()
    return () => {
      cancelled = true
      if (timer) window.clearTimeout(timer)
    }
  }, [checkOrder, props.open, props.tradeNo, updateState])

  useEffect(() => {
    if (!props.open || !props.expiresAt) {
      setRemaining('')
      return
    }

    const expiresAt = props.expiresAt
    let timer: number | undefined
    const updateRemaining = () => {
      const seconds = Math.max(0, expiresAt - Math.floor(Date.now() / 1000))
      if (seconds === 0) {
        setRemaining('')
        if (!isTerminalPaymentState(stateRef.current)) void checkOrder()
        return
      }
      const minutes = Math.floor(seconds / 60)
      setRemaining(`${minutes}:${String(seconds % 60).padStart(2, '0')}`)
      timer = window.setTimeout(updateRemaining, 1000)
    }

    updateRemaining()
    return () => {
      if (timer) window.clearTimeout(timer)
    }
  }, [checkOrder, props.expiresAt, props.open])

  let statusContent: ReactNode
  if (state === 'success') {
    statusContent = (
      <Alert>
        <HugeiconsIcon icon={CheckmarkCircle02Icon} />
        <AlertDescription>{t('Payment completed')}</AlertDescription>
      </Alert>
    )
  } else if (state === 'failed') {
    statusContent = (
      <Alert variant='destructive'>
        <HugeiconsIcon icon={MultiplicationSignCircleIcon} />
        <AlertDescription>{t('Payment failed')}</AlertDescription>
      </Alert>
    )
  } else if (state === 'expired') {
    statusContent = (
      <Alert variant='destructive'>
        <HugeiconsIcon icon={MultiplicationSignCircleIcon} />
        <AlertDescription>{t('Payment order expired')}</AlertDescription>
      </Alert>
    )
  } else if (state === 'error') {
    statusContent = (
      <Alert variant='destructive'>
        <HugeiconsIcon icon={MultiplicationSignCircleIcon} />
        <AlertDescription>
          {t('Unable to check payment status')}
        </AlertDescription>
      </Alert>
    )
  } else {
    statusContent = (
      <Alert>
        <HugeiconsIcon icon={InformationCircleIcon} />
        <AlertDescription>
          {t('Scan the QR code with WeChat to complete payment.')}
        </AlertDescription>
      </Alert>
    )
  }

  let paymentVisual: ReactNode
  if (state === 'pending' || state === 'error') {
    paymentVisual = (
      <div className='rounded-md border bg-white p-4'>
        <QRCodeSVG value={props.codeUrl} size={220} level='M' includeMargin />
      </div>
    )
  } else {
    paymentVisual = (
      <HugeiconsIcon
        icon={
          state === 'success'
            ? CheckmarkCircle02Icon
            : MultiplicationSignCircleIcon
        }
        className={
          state === 'success'
            ? 'text-success size-20'
            : 'text-destructive size-20'
        }
        aria-hidden='true'
      />
    )
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('WeChat Pay')}
      description={t(
        'Use WeChat to scan the QR code. This page checks the payment status automatically.'
      )}
      contentClassName='max-sm:w-[calc(100vw-1.5rem)] sm:max-w-md'
      bodyClassName='flex flex-col gap-4'
      footer={
        <Button variant='outline' onClick={() => props.onOpenChange(false)}>
          {t('Close')}
        </Button>
      }
    >
      <div className='flex flex-col items-center gap-4'>
        {paymentVisual}
        {props.amount ? (
          <p className='text-lg font-semibold'>
            {props.amount} {props.currency ?? 'CNY'}
          </p>
        ) : null}
        {remaining && (state === 'pending' || state === 'error') ? (
          <p className='text-muted-foreground text-sm'>
            {t('Order expires in')} {remaining}
          </p>
        ) : null}
        {statusContent}
        <div className='flex w-full items-center gap-2 rounded-md border px-3 py-2 text-xs'>
          <span className='text-muted-foreground min-w-0 flex-1 truncate'>
            {props.tradeNo}
          </span>
          <CopyButton
            value={props.tradeNo}
            size='icon'
            variant='ghost'
            aria-label={t('Copy order number')}
            tooltip={t('Copy order number')}
          />
        </div>
        {(state === 'pending' || state === 'error') && (
          <Button
            variant='ghost'
            size='sm'
            onClick={() => void checkOrder()}
            disabled={checking}
          >
            {checking && <Spinner data-icon='inline-start' />}
            {t('Check payment status')}
          </Button>
        )}
      </div>
    </Dialog>
  )
}
