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
import { zodResolver } from '@hookform/resolvers/zod'
import { SentIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'

import {
  TICKET_CONTENT_MAX_LENGTH,
  TICKET_MESSAGE_MAX_COUNT,
} from '../constants'
import {
  getTicketReplySchema,
  truncateTicketText,
  type TicketReplyFormValues,
} from '../lib/ticket-form'
import type { TicketStatus, TicketWritePayload } from '../types'
import { TicketImageInput } from './ticket-attachment'

export function TicketReplyForm(props: {
  audience: 'user' | 'admin'
  status: TicketStatus
  messageCount: number
  isSubmitting: boolean
  onSubmit: (payload: TicketWritePayload) => Promise<boolean>
}) {
  const { t } = useTranslation()
  const schema = useMemo(() => getTicketReplySchema(t), [t])
  const form = useForm<TicketReplyFormValues>({
    resolver: zodResolver(schema),
    defaultValues: { content: '' },
  })
  const [image, setImage] = useState<File | null>(null)
  const content = form.watch('content')
  const contentField = form.register('content')
  const expectedStatus =
    props.audience === 'admin' ? 'waiting_admin' : 'waiting_user'
  const turnAllowed = props.status === expectedStatus
  const messageLimitReached = props.messageCount >= TICKET_MESSAGE_MAX_COUNT
  const canReply = turnAllowed && !messageLimitReached

  if (!canReply) {
    let message = t('This ticket is closed.')
    if (messageLimitReached) {
      message = t('This ticket has reached the message limit.')
    } else if (props.status !== 'closed') {
      message =
        props.audience === 'admin'
          ? t('Waiting for the user to reply.')
          : t('Waiting for support to reply.')
    }
    return (
      <div className='border-border/70 border-t p-3 sm:p-4'>
        <Alert>
          <AlertDescription>{message}</AlertDescription>
        </Alert>
      </div>
    )
  }

  return (
    <form
      className='border-border/70 flex shrink-0 flex-col gap-2 border-t p-3 sm:p-4'
      onSubmit={form.handleSubmit(async (values) => {
        try {
          const success = await props.onSubmit({ ...values, image })
          if (success) {
            form.reset()
            setImage(null)
          }
        } catch {
          // Global API and mutation handlers own error notifications.
        }
      })}
    >
      <FieldGroup className='gap-2'>
        <Field data-invalid={Boolean(form.formState.errors.content)}>
          <FieldLabel htmlFor='ticket-reply'>{t('Reply')}</FieldLabel>
          <Textarea
            id='ticket-reply'
            rows={3}
            maxLength={TICKET_CONTENT_MAX_LENGTH * 2}
            disabled={props.isSubmitting}
            aria-invalid={Boolean(form.formState.errors.content)}
            placeholder={t('Write a reply...')}
            className='max-h-40 min-h-20 resize-y [overflow-wrap:anywhere]'
            {...contentField}
            onChange={(event) => {
              event.currentTarget.value = truncateTicketText(
                event.currentTarget.value,
                TICKET_CONTENT_MAX_LENGTH
              )
              void contentField.onChange(event)
            }}
          />
          <div className='flex items-start justify-between gap-3'>
            <FieldError errors={[form.formState.errors.content]} />
            <span className='text-muted-foreground ms-auto shrink-0 text-xs'>
              {[...content].length}/{TICKET_CONTENT_MAX_LENGTH}
            </span>
          </div>
        </Field>
        <Field orientation='horizontal' className='flex-wrap items-end'>
          <TicketImageInput
            file={image}
            onFileChange={setImage}
            disabled={props.isSubmitting}
          />
          <Button
            type='submit'
            className='ms-auto'
            disabled={props.isSubmitting || !content.trim()}
          >
            {props.isSubmitting ? (
              <Spinner data-icon='inline-start' />
            ) : (
              <HugeiconsIcon icon={SentIcon} data-icon='inline-start' />
            )}
            {t('Send reply')}
          </Button>
        </Field>
      </FieldGroup>
    </form>
  )
}
