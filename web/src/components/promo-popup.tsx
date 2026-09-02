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
import { useEffect, useMemo, useState } from 'react'
import { Megaphone, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { useStatus } from '@/hooks/use-status'
import { Button } from '@/components/ui/button'
import { Markdown } from '@/components/ui/markdown'

const DISMISS_KEY_PREFIX = 'promo_popup_dismissed'

function hashString(input: string): string {
  let hash = 0
  if (!input) return '0'

  for (let i = 0; i < input.length; i += 1) {
    hash = (hash << 5) - hash + input.charCodeAt(i)
    hash |= 0
  }

  return Math.abs(hash).toString(36)
}

function isDismissed(key: string): boolean {
  try {
    return window.localStorage.getItem(key) === 'true'
  } catch {
    return false
  }
}

export function PromoPopup() {
  const { t } = useTranslation()
  const { status } = useStatus()
  const content = String(status?.promo_popup_content ?? '').trim()
  const contentHash = useMemo(() => hashString(content), [content])
  const dismissKey = `${DISMISS_KEY_PREFIX}:${contentHash}`
  const [visible, setVisible] = useState(false)

  useEffect(() => {
    if (!content) {
      setVisible(false)
      return
    }

    setVisible(!isDismissed(dismissKey))
  }, [content, dismissKey])

  if (!content || !visible) return null

  const handleClose = () => {
    try {
      window.localStorage.setItem(dismissKey, 'true')
    } catch {
      /* empty */
    }
    setVisible(false)
  }

  return (
    <aside
      className={cn(
        'fixed right-4 bottom-4 z-50 w-[min(24rem,calc(100vw-2rem))]',
        'bg-card/95 text-card-foreground rounded-xl border shadow-xl backdrop-blur',
        'animate-in fade-in slide-in-from-bottom-3 duration-300'
      )}
      aria-label={t('Promotional popup')}
    >
      <div className='bg-primary h-1 rounded-t-xl' />
      <div className='flex items-start gap-3 p-4'>
        <div className='bg-primary/10 text-primary mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-lg'>
          <Megaphone aria-hidden='true' />
        </div>
        <div className='min-w-0 flex-1'>
          <div className='mb-2 flex items-center justify-between gap-3'>
            <div className='text-sm font-semibold'>
              {t('Promotional popup')}
            </div>
            <Button
              type='button'
              variant='ghost'
              size='icon'
              className='-mt-1 -mr-1 size-8 shrink-0'
              onClick={handleClose}
              aria-label={t('Close promotional popup')}
            >
              <X />
            </Button>
          </div>
          <div className='max-h-[min(52vh,22rem)] overflow-y-auto pr-1'>
            <Markdown className='prose-sm'>{content}</Markdown>
          </div>
        </div>
      </div>
    </aside>
  )
}
