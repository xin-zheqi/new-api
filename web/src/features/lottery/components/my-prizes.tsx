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
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Copy } from 'lucide-react'
import { toast } from 'sonner'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { getMyLotteryPrizes } from '../api'

export function MyPrizes() {
  const { t } = useTranslation()
  const prizesQuery = useQuery({
    queryKey: ['my-lottery-prizes'],
    queryFn: () => getMyLotteryPrizes({ p: 1, page_size: 50 }),
  })
  const prizes = prizesQuery.data?.data.items ?? []

  return (
    <div className='grid gap-3'>
      {prizes.map((prize) => (
        <Card key={prize.id}>
          <CardContent className='flex flex-col gap-2 p-4 md:flex-row md:items-center md:justify-between'>
            <div className='min-w-0'>
              <Tooltip>
                <TooltipTrigger render={<div className='truncate font-medium' />}>
                  {prize.title}
                </TooltipTrigger>
                <TooltipContent>{prize.title}</TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger render={<div className='text-muted-foreground truncate text-sm' />}>
                  {prize.prize_name}
                </TooltipTrigger>
                <TooltipContent>{prize.prize_name}</TooltipContent>
              </Tooltip>
            </div>
            <div className='flex items-center gap-2'>
              <div className='bg-muted max-w-64 truncate rounded-lg px-3 py-2 font-mono text-sm'>
                {prize.code}
              </div>
              <Button
                variant='outline'
                size='icon'
                aria-label={t('Copy redemption code')}
                onClick={async () => {
                  await navigator.clipboard.writeText(prize.code)
                  toast.success(t('Copied'))
                }}
              >
                <Copy data-icon='inline-start' />
              </Button>
            </div>
          </CardContent>
        </Card>
      ))}
      {!prizesQuery.isLoading && prizes.length === 0 && (
        <Card>
          <CardHeader>
            <CardTitle>{t('No prizes yet')}</CardTitle>
            <CardDescription>
              {t('Your winning prizes will appear here after draws finish.')}
            </CardDescription>
          </CardHeader>
        </Card>
      )}
    </div>
  )
}
