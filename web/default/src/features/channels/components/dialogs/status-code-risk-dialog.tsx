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
import { AlertTriangle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'

type StatusCodeRiskDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  detailItems: string[]
  onConfirm: () => void
}

export function StatusCodeRiskDialog({
  open,
  onOpenChange,
  detailItems,
  onConfirm,
}: StatusCodeRiskDialogProps) {
  const { t } = useTranslation()

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogMedia className='text-amber-600'>
            <AlertTriangle />
          </AlertDialogMedia>
          <AlertDialogTitle>{t('Confirm status code mapping')}</AlertDialogTitle>
          <AlertDialogDescription render={<div />}>
            <div className='space-y-3'>
              <p>
                {t(
                  'The following mappings convert upstream error status codes into non-error status codes. This can make failed upstream requests look successful to clients.'
                )}
              </p>
              <ul className='bg-muted/40 max-h-40 list-disc space-y-1 overflow-auto rounded-md px-5 py-3 font-mono text-xs'>
                {detailItems.map((item) => (
                  <li key={item}>{item}</li>
                ))}
              </ul>
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
          <AlertDialogAction onClick={onConfirm}>
            {t('Continue')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
