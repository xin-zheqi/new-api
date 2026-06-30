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
import { Gift, Sparkles, Trophy } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Main } from '@/components/layout'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'
import { LotteryActivities } from './components/lottery-activities'
import { LotteryAdmin } from './components/lottery-admin'
import { MyPrizes } from './components/my-prizes'

export function LotteryPage() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const isAdmin = Boolean(user && user.role >= ROLE.ADMIN)

  return (
    <Main className='overflow-auto'>
      <div className='mx-auto flex w-full max-w-7xl flex-col gap-5 p-4 md:p-6'>
        <div className='flex flex-col gap-2 md:flex-row md:items-end md:justify-between'>
          <div className='flex flex-col gap-1'>
            <h1 className='text-2xl font-semibold tracking-normal'>
              {t('Lucky Draw')}
            </h1>
            <p className='text-muted-foreground text-sm'>
              {t('Join active draws, track participants, and claim your prizes.')}
            </p>
          </div>
        </div>

        <Tabs defaultValue='activities'>
          <TabsList>
            <TabsTrigger value='activities'>
              <Sparkles data-icon='inline-start' />
              {t('Active draws')}
            </TabsTrigger>
            <TabsTrigger value='prizes'>
              <Gift data-icon='inline-start' />
              {t('My prizes')}
            </TabsTrigger>
            {isAdmin && (
              <TabsTrigger value='admin'>
                <Trophy data-icon='inline-start' />
                {t('Draw management')}
              </TabsTrigger>
            )}
          </TabsList>

          <TabsContent value='activities'>
            <LotteryActivities />
          </TabsContent>
          <TabsContent value='prizes'>
            <MyPrizes />
          </TabsContent>
          {isAdmin && (
            <TabsContent value='admin'>
              <LotteryAdmin />
            </TabsContent>
          )}
        </Tabs>
      </div>
    </Main>
  )
}
