import { ExternalLink, Store } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader } from '@/components/ui/card'

export function MallSection({ url }: { url?: string }) {
  const { t } = useTranslation()
  if (!url) return null

  const sandboxPermissions =
    new URL(url).origin === window.location.origin
      ? 'allow-forms allow-popups allow-popups-to-escape-sandbox allow-scripts'
      : 'allow-forms allow-popups allow-popups-to-escape-sandbox allow-scripts allow-same-origin allow-storage-access-by-user-activation'

  return (
    <Card className='overflow-hidden'>
      <CardHeader className='flex flex-row items-center justify-between border-b p-4'>
        <div className='flex items-center gap-2 font-medium'><Store className='h-4 w-4' />{t('Mall')}</div>
        <Button variant='outline' size='sm' render={<a href={url} target='_blank' rel='noopener noreferrer' />}>
          <ExternalLink className='mr-2 h-4 w-4' />{t('Open mall')}
        </Button>
      </CardHeader>
      <CardContent className='p-0'>
        <iframe
          src={url}
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
