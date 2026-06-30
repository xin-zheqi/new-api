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
import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { createLottery, updateLottery } from '../api'
import { defaultLotteryForm, WEEKDAYS } from '../constants'
import type { CreateLotteryPayload, LotteryActivity, LotteryMode } from '../types'
import {
  parsePrizeCodes,
  toDateTimeLocal,
  toUnixSeconds,
  weekdayLabel,
} from '../utils'
import { Field } from './field'

export function CreateLotteryDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  lottery?: LotteryActivity | null
  readOnly?: boolean
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [form, setForm] = useState(defaultLotteryForm)
  const editing = Boolean(props.lottery)
  const readOnly = Boolean(props.readOnly)
  const prizeCodes = useMemo(
    () => parsePrizeCodes(form.prizeCodes),
    [form.prizeCodes]
  )
  const requiredCodes = form.winnerCount * form.prizePerWinner

  useEffect(() => {
    if (!props.open) return
    if (!props.lottery) {
      setForm(defaultLotteryForm)
      return
    }
    const lottery = props.lottery
    setForm({
      title: lottery.title,
      description: lottery.description ?? '',
      prizeName: lottery.prize_name,
      mode: lottery.mode,
      winnerCount: lottery.winner_count,
      prizePerWinner: lottery.prize_per_winner,
      registrationStart: toDateTimeLocal(lottery.round?.registration_start),
      registrationEnd: toDateTimeLocal(lottery.round?.registration_end),
      drawTime: toDateTimeLocal(lottery.round?.draw_time),
      scheduleWeekdays: lottery.schedule_weekdays ?? [1, 3, 5],
      scheduleStartTime: lottery.schedule_start_time || '09:00',
      scheduleEndTime: lottery.schedule_end_time || '18:00',
      scheduleDrawTime: lottery.schedule_draw_time || '20:00',
      prizeCodes: (lottery.prize_codes ?? []).join('\n'),
    })
  }, [props.open, props.lottery])

  const createMutation = useMutation({
    mutationFn: createLottery,
    onSuccess: async (res) => {
      if (res.success) {
        toast.success(t('Draw created'))
        props.onOpenChange(false)
        setForm(defaultLotteryForm)
        await queryClient.invalidateQueries({ queryKey: ['admin-lotteries'] })
        await queryClient.invalidateQueries({ queryKey: ['lotteries'] })
      }
    },
  })
  const updateMutation = useMutation({
    mutationFn: (payload: CreateLotteryPayload) =>
      updateLottery(props.lottery?.id ?? 0, payload),
    onSuccess: async (res) => {
      if (res.success) {
        toast.success(t('Draw updated'))
        props.onOpenChange(false)
        await queryClient.invalidateQueries({ queryKey: ['admin-lotteries'] })
        await queryClient.invalidateQueries({ queryKey: ['lotteries'] })
      }
    },
  })

  const update = <K extends keyof typeof defaultLotteryForm>(
    key: K,
    value: (typeof defaultLotteryForm)[K]
  ) => setForm((current) => ({ ...current, [key]: value }))

  const submit = () => {
    const payload: CreateLotteryPayload = {
      title: form.title,
      description: form.description,
      prize_name: form.prizeName,
      mode: form.mode,
      winner_count: form.winnerCount,
      prize_per_winner: form.prizePerWinner,
      registration_start: toUnixSeconds(form.registrationStart),
      registration_end: toUnixSeconds(form.registrationEnd),
      draw_time: toUnixSeconds(form.drawTime),
      schedule_weekdays: form.scheduleWeekdays,
      schedule_start_time: form.scheduleStartTime,
      schedule_end_time: form.scheduleEndTime,
      schedule_draw_time: form.scheduleDrawTime,
      prize_codes: prizeCodes,
    }
    if (editing) {
      if (readOnly) return
      updateMutation.mutate(payload)
      return
    }
    createMutation.mutate(payload)
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-h-[90vh] overflow-auto sm:max-w-3xl'>
        <DialogHeader>
          <DialogTitle>
            {readOnly ? t('Details') : editing ? t('Edit draw') : t('Create draw')}
          </DialogTitle>
          <DialogDescription>
            {t('Keep the schedule, eligibility, and prize codes in one isolated draw configuration.')}
          </DialogDescription>
        </DialogHeader>
        <div className='flex flex-col gap-6'>
          <section className='flex flex-col gap-3'>
            <div>
              <h3 className='text-sm font-medium'>{t('Basic information')}</h3>
              <p className='text-muted-foreground text-sm'>
                {t('Name the activity and describe what users are joining.')}
              </p>
            </div>
            <div className='grid gap-3 md:grid-cols-2'>
            <Field label={t('Draw title')}>
              <Input
                value={form.title}
                disabled={readOnly}
                onChange={(event) => update('title', event.target.value)}
              />
            </Field>
            <Field label={t('Prize name')}>
              <Input
                value={form.prizeName}
                disabled={readOnly}
                onChange={(event) => update('prizeName', event.target.value)}
              />
            </Field>
            </div>
            <Field label={t('Description')}>
              <Textarea
                value={form.description}
                disabled={readOnly}
                onChange={(event) => update('description', event.target.value)}
                className='min-h-24'
              />
            </Field>
          </section>

          <section className='flex flex-col gap-3'>
            <div>
              <h3 className='text-sm font-medium'>{t('Draw rules')}</h3>
              <p className='text-muted-foreground text-sm'>
                {t('Undrawn activities can be adjusted before winners are generated.')}
              </p>
            </div>
            <div className='grid gap-3 md:grid-cols-3'>
            <Field label={t('Mode')}>
              <NativeSelect
                value={form.mode}
                disabled={readOnly}
                onChange={(event) => update('mode', event.target.value as LotteryMode)}
                className='w-full'
              >
                <NativeSelectOption value='once'>{t('One-time')}</NativeSelectOption>
                <NativeSelectOption value='scheduled'>{t('Scheduled')}</NativeSelectOption>
              </NativeSelect>
            </Field>
            <Field label={t('Winner count')}>
              <Input
                type='number'
                min={1}
                value={form.winnerCount}
                disabled={readOnly}
                onChange={(event) =>
                  update('winnerCount', Number(event.target.value) || 1)
                }
              />
            </Field>
            <Field label={t('Prizes per winner')}>
              <Input
                type='number'
                min={1}
                value={form.prizePerWinner}
                disabled={readOnly}
                onChange={(event) =>
                  update('prizePerWinner', Number(event.target.value) || 1)
                }
              />
            </Field>
            </div>
          </section>

          {form.mode === 'once' ? (
            <section className='flex flex-col gap-3'>
              <h3 className='text-sm font-medium'>{t('One-time schedule')}</h3>
              <div className='grid gap-3 md:grid-cols-3'>
              <Field label={t('Registration start')}>
                <Input
                  type='datetime-local'
                  value={form.registrationStart}
                  disabled={readOnly}
                  onChange={(event) => update('registrationStart', event.target.value)}
                />
              </Field>
              <Field label={t('Registration end')}>
                <Input
                  type='datetime-local'
                  value={form.registrationEnd}
                  disabled={readOnly}
                  onChange={(event) => update('registrationEnd', event.target.value)}
                />
              </Field>
              <Field label={t('Draw time')}>
                <Input
                  type='datetime-local'
                  value={form.drawTime}
                  disabled={readOnly}
                  onChange={(event) => update('drawTime', event.target.value)}
                />
              </Field>
              </div>
            </section>
          ) : (
            <section className='flex flex-col gap-3'>
              <h3 className='text-sm font-medium'>{t('Recurring schedule')}</h3>
              <div className='flex flex-wrap gap-2'>
                {WEEKDAYS.map((day) => (
                  <label
                    key={day}
                    className='border-input flex items-center gap-2 rounded-lg border px-3 py-2 text-sm'
                  >
                    <Checkbox
                      checked={form.scheduleWeekdays.includes(day)}
                      disabled={readOnly}
                      onCheckedChange={(checked) => {
                        const next = checked === true
                          ? [...form.scheduleWeekdays, day]
                          : form.scheduleWeekdays.filter((value) => value !== day)
                        update(
                          'scheduleWeekdays',
                          [...new Set(next)].sort((a, b) => a - b)
                        )
                      }}
                    />
                    {weekdayLabel(day, t)}
                  </label>
                ))}
              </div>
              <div className='grid gap-3 md:grid-cols-3'>
                <Field label={t('Registration start time')}>
                  <Input
                    type='time'
                    value={form.scheduleStartTime}
                    disabled={readOnly}
                    onChange={(event) => update('scheduleStartTime', event.target.value)}
                  />
                </Field>
                <Field label={t('Registration end time')}>
                  <Input
                    type='time'
                    value={form.scheduleEndTime}
                    disabled={readOnly}
                    onChange={(event) => update('scheduleEndTime', event.target.value)}
                  />
                </Field>
                <Field label={t('Scheduled draw time')}>
                  <Input
                    type='time'
                    value={form.scheduleDrawTime}
                    disabled={readOnly}
                    onChange={(event) => update('scheduleDrawTime', event.target.value)}
                  />
                </Field>
              </div>
            </section>
          )}

          <section className='flex flex-col gap-3'>
            <div>
              <h3 className='text-sm font-medium'>{t('Prize codes')}</h3>
              <p className='text-muted-foreground text-sm'>
                {t('Import one redemption code per line. Editing replaces all unassigned codes.')}
              </p>
            </div>
          <Field
            label={t('Redemption codes')}
            hint={t('{{current}} codes imported, {{required}} required per draw', {
              current: prizeCodes.length,
              required: requiredCodes,
            })}
          >
            <Textarea
              value={form.prizeCodes}
              disabled={readOnly}
              onChange={(event) => update('prizeCodes', event.target.value)}
              className='min-h-40 font-mono'
              placeholder={'CODE-001\nCODE-002\nCODE-003'}
            />
          </Field>
          </section>
        </div>
        <DialogFooter>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {readOnly ? t('Close') : t('Cancel')}
          </Button>
          {!readOnly && (
            <Button
              onClick={submit}
              disabled={createMutation.isPending || updateMutation.isPending}
            >
              {editing ? t('Save changes') : t('Create draw')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
