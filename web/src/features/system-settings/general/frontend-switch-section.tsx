/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Monitor } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'

import { useUpdateOption } from '../hooks/use-update-option'

type FrontendSwitchSectionProps = {
  disabled?: boolean
}

export function FrontendSwitchSection(props: FrontendSwitchSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [confirmOpen, setConfirmOpen] = useState(false)

  const switchToClassicFrontend = () => {
    updateOption.mutate(
      { key: 'theme.frontend', value: 'classic' },
      {
        onSuccess: () => {
          setConfirmOpen(false)
          window.location.replace('/')
        },
      }
    )
  }

  const isDisabled = props.disabled || updateOption.isPending

  return (
    <>
      <div className='flex min-w-0 flex-col gap-3 rounded-lg border p-4 sm:flex-row sm:items-center sm:justify-between'>
        <div className='flex min-w-0 items-start gap-3'>
          <Monitor
            className='text-muted-foreground mt-0.5 size-4 shrink-0'
            aria-hidden='true'
          />
          <div className='min-w-0 space-y-1'>
            <p className='text-sm font-medium'>{t('Frontend interface')}</p>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Switching will reload the page and open the classic frontend. Save or reset unsaved changes before continuing.'
              )}
            </p>
          </div>
        </div>
        <Button
          type='button'
          variant='outline'
          size='sm'
          disabled={isDisabled}
          onClick={() => setConfirmOpen(true)}
        >
          <Monitor data-icon='inline-start' aria-hidden='true' />
          {t('Switch to classic frontend')}
        </Button>
      </div>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={(open) => {
          if (!updateOption.isPending) setConfirmOpen(open)
        }}
        title={t('Switch to classic frontend?')}
        desc={t(
          'Switching will reload the page and open the classic frontend. Save or reset unsaved changes before continuing.'
        )}
        confirmText={t('Switch to classic frontend')}
        cancelBtnText={t('Cancel')}
        isLoading={updateOption.isPending}
        handleConfirm={switchToClassicFrontend}
      />
    </>
  )
}
