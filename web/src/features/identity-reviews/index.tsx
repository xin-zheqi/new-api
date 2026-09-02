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
import { Check, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { api } from '@/lib/api'

type ReviewUser = {
  id: number
  username: string
  display_name?: string
  email?: string
  identity_requested?: string
}

export function IdentityReviews() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: ['identity-reviews'],
    queryFn: async () =>
      (await api.get('/api/user/identity-reviews')).data.data as ReviewUser[],
  })
  const review = useMutation({
    mutationFn: ({
      id,
      action,
    }: {
      id: number
      action: 'approve' | 'reject'
    }) => api.post(`/api/user/identity-reviews/${id}/${action}`),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ['identity-reviews'] }),
  })
  const label = (identity?: string) => {
    if (identity === 'university') return t('University')
    if (identity === 'enterprise') return t('Enterprise')
    if (identity === 'student') return t('Student')
    if (identity === 'personal') return t('Personal')
    return '-'
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Identity Review')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto flex w-full max-w-7xl flex-col py-2'>
          <p className='text-muted-foreground mb-6 text-sm'>
            {t('University and enterprise identity applications')}
          </p>
          <div className='flex flex-col gap-3'>
            {query.data?.map((user) => (
              <Card key={user.id}>
                <CardContent className='flex flex-wrap items-center gap-3 p-4'>
                  <div className='min-w-0 flex-1'>
                    <p className='font-medium'>
                      {user.display_name || user.username}
                    </p>
                    <p className='text-muted-foreground text-xs'>
                      {user.email || user.username}
                    </p>
                  </div>
                  <Badge variant='secondary'>
                    {label(user.identity_requested)}
                  </Badge>
                  <Button
                    size='sm'
                    onClick={() =>
                      review.mutate({ id: user.id, action: 'approve' })
                    }
                  >
                    <Check className='mr-1 h-4 w-4' />
                    {t('Approve')}
                  </Button>
                  <Button
                    size='sm'
                    variant='outline'
                    onClick={() =>
                      review.mutate({ id: user.id, action: 'reject' })
                    }
                  >
                    <X className='mr-1 h-4 w-4' />
                    {t('Reject')}
                  </Button>
                </CardContent>
              </Card>
            ))}
            {!query.data?.length && (
              <p className='text-muted-foreground text-sm'>
                {t('No identity applications')}
              </p>
            )}
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
