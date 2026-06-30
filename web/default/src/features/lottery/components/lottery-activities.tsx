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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { getLotteries, getLotterySettings, joinLottery } from '../api'
import type { LotteryDrawStatusFilter } from '../types'
import { EligibilityStatus } from './eligibility-status'
import { LotteryCard } from './lottery-card'

export function LotteryActivities() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [drawStatus, setDrawStatus] = useState<LotteryDrawStatusFilter>('all')

  const settingsQuery = useQuery({
    queryKey: ['lottery-settings'],
    queryFn: getLotterySettings,
  })
  const lotteriesQuery = useQuery({
    queryKey: ['lotteries', drawStatus],
    queryFn: () => getLotteries({ draw_status: drawStatus }),
    refetchInterval: 10000,
  })
  const joinMutation = useMutation({
    mutationFn: joinLottery,
    onSuccess: async (res) => {
      if (res.success) {
        toast.success(t('Joined draw successfully'))
        await queryClient.invalidateQueries({ queryKey: ['lotteries'] })
      }
    },
  })

  if (settingsQuery.data?.data && !settingsQuery.data.data.enabled) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t('Lucky draw is disabled')}</CardTitle>
          <CardDescription>
            {t('The administrator has not enabled lucky draw activities yet.')}
          </CardDescription>
        </CardHeader>
      </Card>
    )
  }

  const lotteries = lotteriesQuery.data?.data ?? []
  const eligibility = settingsQuery.data?.data?.eligibility

  return (
    <div className='flex flex-col gap-4'>
      {eligibility && (
        <EligibilityStatus status={eligibility} />
      )}

      <div className='flex flex-col gap-3 md:flex-row md:items-center md:justify-between'>
        <div>
          <h2 className='text-base font-medium'>{t('Draw activities')}</h2>
          <p className='text-muted-foreground text-sm'>
            {t('Activities are sorted by draw time from nearest to latest.')}
          </p>
        </div>
        <NativeSelect
          value={drawStatus}
          onChange={(event) =>
            setDrawStatus(event.target.value as LotteryDrawStatusFilter)
          }
          className='md:w-48'
        >
          <NativeSelectOption value='all'>{t('All draws')}</NativeSelectOption>
          <NativeSelectOption value='undrawn'>{t('Undrawn')}</NativeSelectOption>
          <NativeSelectOption value='drawn'>{t('Drawn')}</NativeSelectOption>
        </NativeSelect>
      </div>

      <div className='grid gap-4 md:grid-cols-2 xl:grid-cols-3'>
        {lotteries.map((lottery) => (
          <LotteryCard
            key={lottery.id}
            lottery={lottery}
            onJoin={() => joinMutation.mutate(lottery.id)}
            joining={joinMutation.isPending}
          />
        ))}
        {!lotteriesQuery.isLoading && lotteries.length === 0 && (
          <Card className='md:col-span-2 xl:col-span-3'>
            <CardHeader>
              <CardTitle>{t('No matching draws')}</CardTitle>
              <CardDescription>
                {t('Try changing the draw status filter.')}
              </CardDescription>
            </CardHeader>
          </Card>
        )}
      </div>
    </div>
  )
}
