import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, FileUp, Trash2 } from 'lucide-react'
import { useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Main } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { api } from '@/lib/api'
import { formatQuota } from '@/lib/format'

import type { InvoiceApplication } from './types'

export function AdminInvoiceCenter() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const uploadTarget = useRef<number | null>(null)
  const fileInput = useRef<HTMLInputElement>(null)
  const query = useQuery({
    queryKey: ['admin-invoices'],
    queryFn: async () =>
      (await api.get('/api/invoice/admin/applications')).data
        .data as InvoiceApplication[],
  })
  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ['admin-invoices'] })
  const upload = useMutation({
    mutationFn: async ({ id, file }: { id: number; file: File }) => {
      const form = new FormData()
      form.append('file', file)
      return api.post(`/api/invoice/admin/applications/${id}/pdf`, form)
    },
    onSuccess: refresh,
  })
  const remove = useMutation({
    mutationFn: (id: number) =>
      api.delete(`/api/invoice/admin/applications/${id}/pdf`),
    onSuccess: refresh,
  })
  const complete = useMutation({
    mutationFn: (id: number) =>
      api.post(`/api/invoice/admin/applications/${id}/complete`),
    onSuccess: (response) => {
      toast.success(response.data?.message || t('Invoice completed'))
      refresh()
    },
  })

  return (
    <Main>
      <div className='mx-auto flex w-full max-w-7xl flex-col gap-4 overflow-auto p-4 sm:p-6'>
        <div>
          <h1 className='text-2xl font-semibold'>
            {t('Invoice Management Center')}
          </h1>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Review applications, upload PDF invoices, and mark them completed.'
            )}
          </p>
        </div>
        <input
          ref={fileInput}
          type='file'
          accept='application/pdf,.pdf'
          className='hidden'
          onChange={(event) => {
            const file = event.target.files?.[0]
            if (file && uploadTarget.current) {
              upload.mutate({ id: uploadTarget.current, file })
            }
            event.target.value = ''
          }}
        />
        <div className='space-y-3'>
          {query.data?.map((application) => (
            <Card key={application.id}>
              <CardContent className='grid gap-3 p-4 lg:grid-cols-[minmax(180px,1fr)_minmax(220px,1.2fr)_auto_auto] lg:items-center'>
                <div>
                  <p className='font-medium'>
                    {application.user?.display_name ||
                      application.user?.username}
                  </p>
                  <p className='text-muted-foreground text-xs'>
                    {application.user?.email || '-'}
                  </p>
                </div>
                <div>
                  <p className='font-medium'>{application.invoice_title}</p>
                  <p className='text-muted-foreground text-xs'>
                    {t('Invoice total amount')}:{' '}
                    {formatQuota(application.total_amount)}
                  </p>
                  <p className='text-muted-foreground text-xs'>
                    {new Date(application.created_at * 1000).toLocaleString()}
                  </p>
                </div>
                <Badge
                  variant={
                    application.status === 'completed' ? 'default' : 'secondary'
                  }
                >
                  {t(
                    application.status === 'completed' ? 'Completed' : 'Pending'
                  )}
                </Badge>
                <div className='flex flex-wrap justify-end gap-2'>
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={() => {
                      uploadTarget.current = application.id
                      fileInput.current?.click()
                    }}
                  >
                    <FileUp />
                    {application.pdf_name ? t('Replace PDF') : t('Upload PDF')}
                  </Button>
                  {application.pdf_name && (
                    <Button
                      variant='outline'
                      size='icon'
                      title={t('Delete PDF')}
                      onClick={() => remove.mutate(application.id)}
                    >
                      <Trash2 />
                    </Button>
                  )}
                  {application.status === 'pending' && (
                    <Button
                      size='sm'
                      disabled={!application.pdf_name || complete.isPending}
                      onClick={() => complete.mutate(application.id)}
                    >
                      <Check />
                      {t('Complete invoice')}
                    </Button>
                  )}
                </div>
              </CardContent>
            </Card>
          ))}
          {!query.data?.length && (
            <p className='text-muted-foreground text-sm'>
              {t('No invoice applications')}
            </p>
          )}
        </div>
      </div>
    </Main>
  )
}
