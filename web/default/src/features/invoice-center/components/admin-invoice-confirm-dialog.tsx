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
import { CheckmarkCircle02Icon, Delete02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Spinner } from '@/components/ui/spinner'

export type InvoiceConfirmAction = 'complete' | 'delete'

export function AdminInvoiceConfirmDialog(props: {
  action: InvoiceConfirmAction | null
  isSubmitting: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}) {
  const { t } = useTranslation()
  const isComplete = props.action === 'complete'

  return (
    <AlertDialog
      open={props.action !== null}
      onOpenChange={(open) => {
        if (!props.isSubmitting) props.onOpenChange(open)
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {isComplete
              ? t('Complete this invoice application?')
              : t('Delete the uploaded PDF?')}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {isComplete
              ? t(
                  'The user will be able to download the PDF and the application can no longer be edited.'
                )
              : t(
                  'The PDF will be removed. You can upload a replacement while the application is pending.'
                )}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={props.isSubmitting}>
            {t('Cancel')}
          </AlertDialogCancel>
          <AlertDialogAction
            variant={isComplete ? 'default' : 'destructive'}
            disabled={props.isSubmitting}
            onClick={props.onConfirm}
          >
            {props.isSubmitting ? (
              <Spinner data-icon='inline-start' />
            ) : (
              <HugeiconsIcon
                icon={isComplete ? CheckmarkCircle02Icon : Delete02Icon}
                data-icon='inline-start'
              />
            )}
            {isComplete ? t('Complete invoice') : t('Delete PDF')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
