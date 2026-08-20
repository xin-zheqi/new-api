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

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'

import {
  TICKET_CONTENT_MAX_LENGTH,
  TICKET_TITLE_MAX_LENGTH,
} from '../constants'
import {
  getTicketCreateSchema,
  truncateTicketText,
  type TicketCreateFormValues,
} from '../lib/ticket-form'
import type { TicketCreatePayload } from '../types'
import { TicketImageInput } from './ticket-attachment'

export function CreateTicketDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  isSubmitting: boolean
  onSubmit: (payload: TicketCreatePayload) => Promise<boolean>
}) {
  const { t } = useTranslation()
  const schema = useMemo(() => getTicketCreateSchema(t), [t])
  const form = useForm<TicketCreateFormValues>({
    resolver: zodResolver(schema),
    defaultValues: { title: '', content: '' },
  })
  const [image, setImage] = useState<File | null>(null)
  const title = form.watch('title')
  const content = form.watch('content')
  const titleField = form.register('title')
  const contentField = form.register('content')

  const reset = () => {
    form.reset()
    setImage(null)
  }
  const close = () => {
    if (props.isSubmitting) return
    reset()
    props.onOpenChange(false)
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!open) {
          close()
          return
        }
        if (!props.isSubmitting) props.onOpenChange(true)
      }}
    >
      <DialogContent className='sm:max-w-xl'>
        <DialogHeader>
          <DialogTitle>{t('Create support ticket')}</DialogTitle>
          <DialogDescription>
            {t('Describe the issue and include one image if it helps.')}
          </DialogDescription>
        </DialogHeader>
        <form
          className='flex min-h-0 flex-col gap-4'
          onSubmit={form.handleSubmit(async (values) => {
            try {
              const success = await props.onSubmit({ ...values, image })
              if (success) {
                reset()
                props.onOpenChange(false)
              }
            } catch {
              // Global API and mutation handlers own error notifications.
            }
          })}
        >
          <FieldGroup>
            <Field data-invalid={Boolean(form.formState.errors.title)}>
              <FieldLabel htmlFor='ticket-title'>{t('Subject')}</FieldLabel>
              <Input
                id='ticket-title'
                maxLength={TICKET_TITLE_MAX_LENGTH * 2}
                disabled={props.isSubmitting}
                aria-invalid={Boolean(form.formState.errors.title)}
                placeholder={t('Briefly summarize the issue')}
                {...titleField}
                onChange={(event) => {
                  event.currentTarget.value = truncateTicketText(
                    event.currentTarget.value,
                    TICKET_TITLE_MAX_LENGTH
                  )
                  void titleField.onChange(event)
                }}
              />
              <div className='flex items-start justify-between gap-3'>
                <FieldError errors={[form.formState.errors.title]} />
                <span className='text-muted-foreground ms-auto shrink-0 text-xs'>
                  {[...title].length}/{TICKET_TITLE_MAX_LENGTH}
                </span>
              </div>
            </Field>
            <Field data-invalid={Boolean(form.formState.errors.content)}>
              <FieldLabel htmlFor='ticket-content'>{t('Message')}</FieldLabel>
              <Textarea
                id='ticket-content'
                rows={7}
                maxLength={TICKET_CONTENT_MAX_LENGTH * 2}
                disabled={props.isSubmitting}
                aria-invalid={Boolean(form.formState.errors.content)}
                placeholder={t('Describe what happened and what you expected')}
                className='max-h-72 min-h-32 resize-y [overflow-wrap:anywhere]'
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
            <Field>
              <FieldLabel>{t('Image')}</FieldLabel>
              <TicketImageInput
                file={image}
                onFileChange={setImage}
                disabled={props.isSubmitting}
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button
              type='button'
              variant='outline'
              disabled={props.isSubmitting}
              onClick={close}
            >
              {t('Cancel')}
            </Button>
            <Button type='submit' disabled={props.isSubmitting}>
              {props.isSubmitting ? (
                <Spinner data-icon='inline-start' />
              ) : (
                <HugeiconsIcon icon={SentIcon} data-icon='inline-start' />
              )}
              {t('Submit ticket')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
