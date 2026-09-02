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
import { Info, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { SectionPageLayout } from '@/components/layout'
import { UserSubscriptionsDialogs } from './components/user-subscriptions-dialogs'
import {
  UserSubscriptionsProvider,
  useUserSubscriptions,
} from './components/user-subscriptions-provider'
import { UserSubscriptionsTable } from './components/user-subscriptions-table'

function UserSubscriptionsContent() {
  const { t } = useTranslation()
  const { setOpen } = useUserSubscriptions()

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {t('Subscription Management')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button onClick={() => setOpen('create')}>
            <Plus className='mr-1 h-4 w-4' />
            {t('Open Subscription')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <Alert className='mb-4'>
            <Info className='h-4 w-4' />
            <AlertDescription>
              {t(
                'All user subscription records are retained for audit. Expired and invalidated subscriptions are read-only.'
              )}
            </AlertDescription>
          </Alert>
          <UserSubscriptionsTable />
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <UserSubscriptionsDialogs />
    </>
  )
}

export function UserSubscriptions() {
  return (
    <UserSubscriptionsProvider>
      <UserSubscriptionsContent />
    </UserSubscriptionsProvider>
  )
}
