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
import { ArrowUpRight01Icon, Store01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader } from '@/components/ui/card'

export function MallSection(props: { url: string }) {
  const { t } = useTranslation()
  const url = new URL(props.url)
  const sandboxPermissions =
    url.origin === window.location.origin
      ? 'allow-forms allow-popups allow-popups-to-escape-sandbox allow-scripts'
      : 'allow-forms allow-popups allow-popups-to-escape-sandbox allow-scripts allow-same-origin allow-storage-access-by-user-activation'

  return (
    <Card className='overflow-hidden'>
      <CardHeader className='flex flex-row items-center justify-between border-b p-4'>
        <div className='flex items-center gap-2 font-medium'>
          <HugeiconsIcon icon={Store01Icon} aria-hidden='true' />
          {t('Mall')}
        </div>
        <Button
          variant='outline'
          size='sm'
          nativeButton={false}
          render={
            <a href={url.href} target='_blank' rel='noopener noreferrer' />
          }
        >
          <HugeiconsIcon
            icon={ArrowUpRight01Icon}
            data-icon='inline-start'
            aria-hidden='true'
          />
          {t('Open mall')}
        </Button>
      </CardHeader>
      <CardContent className='p-0'>
        <iframe
          src={url.href}
          title={t('Mall')}
          className='h-[min(70vh,720px)] w-full border-0'
          sandbox={sandboxPermissions}
          referrerPolicy='no-referrer'
          allow='payment; storage-access'
        />
      </CardContent>
    </Card>
  )
}
