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

import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { useAuthStore } from '@/stores/auth-store'

import { getAuditPolicy } from './api'
import { AuditRecords } from './records'
import { AuditRules } from './rules'

export function ContentAudit() {
  const { t } = useTranslation()
  const owner = useAuthStore((state) => state.auth.user?.id)
  // Private content is never persisted or retained after leaving this page.
  const query = useQuery({
    queryKey: ['content-audit-policy', owner],
    queryFn: getAuditPolicy,
    gcTime: 0,
    retry: false,
  })
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Content audit')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        {!query.data && (
          <p role='status'>
            {query.error
              ? t(
                  'Content audit is unavailable or disabled by the administrator.'
                )
              : t('Loading...')}
          </p>
        )}
        {query.data && (
          <div className='flex flex-col gap-4'>
            <Alert>
              <AlertDescription>
                {t(
                  'Your rules and hit details are private to your account and encrypted at rest. We will do our best to protect the security of your data. Do not send secrets to the network.'
                )}
              </AlertDescription>
            </Alert>
            <p className='text-muted-foreground text-sm'>
              {t(
                  'Records expire after {{days}} days; cleanup runs every 10 minutes.',
                  { days: query.data.retention_days }
              )}
            </p>
            <p className='text-muted-foreground text-sm'>
              {t(
                'Audits text in Chat, Responses (HTTP/WebSocket), Messages, Gemini and supported text fields. Images, audio and file contents are not scanned. Realtime audio is unavailable while your audit policy is active.'
              )}
            </p>
            <Tabs defaultValue='rules'>
              <TabsList>
                <TabsTrigger value='rules'>{t('My rules')}</TabsTrigger>
                <TabsTrigger value='records'>{t('Hit records')}</TabsTrigger>
              </TabsList>
              <TabsContent value='rules'>
                <AuditRules data={query.data} />
              </TabsContent>
              <TabsContent value='records'>
                <AuditRecords />
              </TabsContent>
            </Tabs>
          </div>
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
