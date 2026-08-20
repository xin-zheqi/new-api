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
import {
  Cancel01Icon,
  ImageAdd01Icon,
  ImageNotFound01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useAuthStore } from '@/stores/auth-store'

import { getTicketAttachment } from '../api'
import { TICKET_IMAGE_ACCEPT, ticketQueryKeys } from '../constants'
import { formatAttachmentSize, getTicketImageError } from '../lib/ticket-form'
import type { TicketAttachment } from '../types'

function useAttachmentObjectUrl(
  subjectId: number,
  ticketId: number,
  attachmentId: number,
  enabled: boolean
) {
  const query = useQuery({
    queryKey: ticketQueryKeys.attachment(subjectId, ticketId, attachmentId),
    queryFn: ({ signal }) =>
      getTicketAttachment(ticketId, attachmentId, signal),
    enabled: enabled && subjectId > 0,
    staleTime: Number.POSITIVE_INFINITY,
  })
  const [resource, setResource] = useState<{
    blob: Blob
    url: string
  } | null>(null)

  useEffect(() => {
    if (!query.data) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setResource(null)
      return
    }
    const nextUrl = URL.createObjectURL(query.data)
    // The URL is an external browser resource and must track the cached Blob.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setResource({ blob: query.data, url: nextUrl })
    return () => URL.revokeObjectURL(nextUrl)
  }, [query.data])

  const url = resource && resource.blob === query.data ? resource.url : null
  return { ...query, url }
}

export function TicketAttachmentImage(props: {
  ticketId: number
  attachment: TicketAttachment
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const subjectId = useAuthStore((state) => state.auth.user?.id ?? 0)
  const [shouldLoad, setShouldLoad] = useState(false)
  const [previewOpen, setPreviewOpen] = useState(false)
  const attachmentQuery = useAttachmentObjectUrl(
    subjectId,
    props.ticketId,
    props.attachment.id,
    shouldLoad
  )
  const attachmentQueryKey = ticketQueryKeys.attachment(
    subjectId,
    props.ticketId,
    props.attachment.id
  )

  useEffect(
    () => () => {
      const queryKey = ticketQueryKeys.attachment(
        subjectId,
        props.ticketId,
        props.attachment.id
      )
      void queryClient.cancelQueries({ queryKey, exact: true }).finally(() => {
        queryClient.removeQueries({ queryKey, exact: true })
      })
    },
    [props.attachment.id, props.ticketId, queryClient, subjectId]
  )

  const releaseAttachment = () => {
    setPreviewOpen(false)
    setShouldLoad(false)
    void queryClient
      .cancelQueries({ queryKey: attachmentQueryKey, exact: true })
      .finally(() => {
        queryClient.removeQueries({
          queryKey: attachmentQueryKey,
          exact: true,
        })
      })
  }

  let previewContent
  if (attachmentQuery.isError) {
    previewContent = (
      <div className='text-muted-foreground flex flex-col items-center gap-3 p-6 text-sm'>
        <HugeiconsIcon icon={ImageNotFound01Icon} />
        <span>{t('Image unavailable')}</span>
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={() => void attachmentQuery.refetch()}
        >
          {t('Retry')}
        </Button>
      </div>
    )
  } else if (attachmentQuery.url) {
    previewContent = (
      <img
        src={attachmentQuery.url}
        alt={props.attachment.file_name}
        className='max-h-[75dvh] max-w-full object-contain'
      />
    )
  } else {
    previewContent = <Skeleton className='h-72 w-full' />
  }

  return (
    <>
      <button
        type='button'
        className='border-border bg-muted/30 focus-visible:ring-ring relative block aspect-[4/3] w-40 max-w-full overflow-hidden rounded-md border focus-visible:ring-2 focus-visible:outline-none disabled:cursor-not-allowed'
        aria-label={t('Preview attachment {{name}}', {
          name: props.attachment.file_name,
        })}
        onClick={() => {
          setShouldLoad(true)
          setPreviewOpen(true)
          if (attachmentQuery.isError) void attachmentQuery.refetch()
        }}
      >
        {shouldLoad && attachmentQuery.isLoading ? (
          <Skeleton className='absolute inset-0' />
        ) : null}
        {attachmentQuery.isError && (
          <span className='text-muted-foreground flex size-full flex-col items-center justify-center gap-1 p-2 text-xs'>
            <HugeiconsIcon icon={ImageNotFound01Icon} />
            {t('Image unavailable')}
          </span>
        )}
        {!shouldLoad && (
          <span className='text-muted-foreground flex size-full flex-col items-center justify-center gap-1 p-2 text-xs'>
            <HugeiconsIcon icon={ImageAdd01Icon} />
            {t('View image')}
          </span>
        )}
        {attachmentQuery.url && (
          <img
            src={attachmentQuery.url}
            alt={props.attachment.file_name}
            className='size-full object-cover'
            loading='lazy'
            decoding='async'
          />
        )}
      </button>

      <Dialog
        open={previewOpen}
        onOpenChange={(open) => {
          if (open) {
            setPreviewOpen(true)
            return
          }
          releaseAttachment()
        }}
        title={t('Image preview')}
        description={`${props.attachment.file_name} · ${formatAttachmentSize(props.attachment.size)}`}
        contentClassName='sm:max-w-4xl'
        contentHeight='auto'
      >
        <div className='bg-muted/30 flex max-h-[75dvh] min-h-48 items-center justify-center overflow-hidden rounded-md border'>
          {previewContent}
        </div>
      </Dialog>
    </>
  )
}

export function TicketImageInput(props: {
  file: File | null
  onFileChange: (file: File | null) => void
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const inputRef = useRef<HTMLInputElement>(null)
  const [previewUrl, setPreviewUrl] = useState<string | null>(null)

  useEffect(() => {
    if (!props.file) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setPreviewUrl(null)
      return
    }
    const nextUrl = URL.createObjectURL(props.file)
    // The preview URL is an external browser resource tied to this File.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setPreviewUrl(nextUrl)
    return () => URL.revokeObjectURL(nextUrl)
  }, [props.file])

  return (
    <div className='flex min-w-0 flex-wrap items-center gap-2'>
      <input
        ref={inputRef}
        type='file'
        accept={TICKET_IMAGE_ACCEPT}
        className='hidden'
        disabled={props.disabled}
        aria-label={t('Attach image')}
        onChange={(event) => {
          const file = event.currentTarget.files?.[0]
          event.currentTarget.value = ''
          if (!file) return
          const error = getTicketImageError(file, t)
          if (error) {
            toast.error(error)
            return
          }
          props.onFileChange(file)
        }}
      />
      <Button
        type='button'
        variant='outline'
        size='sm'
        disabled={props.disabled || Boolean(props.file)}
        onClick={() => inputRef.current?.click()}
      >
        <HugeiconsIcon icon={ImageAdd01Icon} data-icon='inline-start' />
        {t('Attach image')}
      </Button>
      {props.file && previewUrl && (
        <div className='border-border flex min-w-0 items-center gap-2 rounded-md border p-1.5'>
          <img
            src={previewUrl}
            alt={props.file.name}
            className='size-9 shrink-0 rounded object-cover'
          />
          <div className='min-w-0'>
            <p className='max-w-40 truncate text-xs font-medium'>
              {props.file.name}
            </p>
            <p className='text-muted-foreground text-xs'>
              {formatAttachmentSize(props.file.size)}
            </p>
          </div>
          <Button
            type='button'
            variant='ghost'
            size='icon-sm'
            disabled={props.disabled}
            aria-label={t('Remove image')}
            title={t('Remove image')}
            onClick={() => props.onFileChange(null)}
          >
            <HugeiconsIcon icon={Cancel01Icon} />
          </Button>
        </div>
      )}
    </div>
  )
}
