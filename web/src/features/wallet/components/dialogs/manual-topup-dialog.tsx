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
import * as z from 'zod'
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { searchUsers, getUsers } from '@/features/users/api'
import type { User } from '@/features/users/types'
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
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { createManualTopup, isApiSuccess } from '../../api'

const createManualTopupSchema = (t: (key: string) => string) =>
  z.object({
    user_id: z.string().min(1, t('Please select a user')),
    payment_method: z
      .string()
      .trim()
      .min(1, t('Payment method is required'))
      .max(50, t('Payment method cannot exceed 50 characters')),
    amount: z.coerce
      .number()
      .positive(t('Recharge amount must be greater than 0')),
    money: z.coerce.number().min(0, t('Payment amount cannot be negative')),
    create_time: z.string().min(1, t('Create time is required')),
    credit_balance: z.boolean(),
  })

type ManualTopupFormInput = z.input<
  ReturnType<typeof createManualTopupSchema>
>
type ManualTopupFormValues = z.output<
  ReturnType<typeof createManualTopupSchema>
>

interface ManualTopupDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: () => void | Promise<void>
}

function toDatetimeLocal(date: Date) {
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(
    date.getDate()
  )}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

function formatUserOption(user: User) {
  return `ID ${user.id} · ${user.username}`
}

function toInputNumberValue(value: unknown) {
  return typeof value === 'number' || typeof value === 'string' ? value : ''
}

export function ManualTopupDialog({
  open,
  onOpenChange,
  onCreated,
}: ManualTopupDialogProps) {
  const { t } = useTranslation()
  const schema = createManualTopupSchema(t)
  const [users, setUsers] = useState<User[]>([])
  const [userKeyword, setUserKeyword] = useState('')
  const [loadingUsers, setLoadingUsers] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  const form = useForm<ManualTopupFormInput, unknown, ManualTopupFormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      user_id: '',
      payment_method: 'bank_transfer',
      amount: 1,
      money: 0,
      create_time: toDatetimeLocal(new Date()),
      credit_balance: false,
    },
  })

  useEffect(() => {
    if (!open) return
    form.reset({
      user_id: '',
      payment_method: 'bank_transfer',
      amount: 1,
      money: 0,
      create_time: toDatetimeLocal(new Date()),
      credit_balance: false,
    })
    setUserKeyword('')
  }, [form, open])

  useEffect(() => {
    if (!open) return

    const timer = window.setTimeout(async () => {
      setLoadingUsers(true)
      try {
        const keyword = userKeyword.trim()
        const response = keyword
          ? await searchUsers({ keyword, p: 1, page_size: 20 })
          : await getUsers({ p: 1, page_size: 20 })
        if (response.success && response.data) {
          setUsers(response.data.items || [])
        } else {
          setUsers([])
        }
      } catch (error) {
        // eslint-disable-next-line no-console
        console.error('Failed to search users:', error)
        setUsers([])
      } finally {
        setLoadingUsers(false)
      }
    }, 250)

    return () => window.clearTimeout(timer)
  }, [open, userKeyword])

  const selectedUserId = form.watch('user_id')
  const selectedUserLabel = useMemo(() => {
    const user = users.find((item) => String(item.id) === selectedUserId)
    return user ? formatUserOption(user) : undefined
  }, [selectedUserId, users])

  const handleSubmit = async (values: ManualTopupFormValues) => {
    const createTime = Math.floor(new Date(values.create_time).getTime() / 1000)
    if (!Number.isFinite(createTime) || createTime <= 0) {
      toast.error(t('Create time is invalid'))
      return
    }

    setSubmitting(true)
    try {
      const response = await createManualTopup({
        user_id: Number(values.user_id),
        payment_method: values.payment_method.trim(),
        amount: values.amount,
        money: values.money,
        create_time: createTime,
        credit_balance: values.credit_balance,
      })
      if (isApiSuccess(response)) {
        toast.success(t('Recharge record created successfully'))
        await onCreated()
        onOpenChange(false)
      } else {
        toast.error(response.message || t('Failed to create recharge record'))
      }
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to create recharge record:', error)
      toast.error(t('Failed to create recharge record'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-[520px]'>
        <DialogHeader>
          <DialogTitle>{t('Create recharge record')}</DialogTitle>
          <DialogDescription>
            {t('Create a successful recharge order for offline payments.')}
          </DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(handleSubmit)}
            className='flex flex-col gap-4'
          >
            <FormField
              control={form.control}
              name='user_id'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('User')}</FormLabel>
                  <FormControl>
                    <div className='flex flex-col gap-2'>
                      <Input
                        value={userKeyword}
                        onChange={(event) => setUserKeyword(event.target.value)}
                        placeholder={t('Search by user ID or username')}
                      />
                      <Select
                        value={field.value}
                        onValueChange={(value) => value && field.onChange(value)}
                      >
                        <SelectTrigger className='w-full'>
                          <SelectValue
                            placeholder={
                              loadingUsers
                                ? t('Loading users...')
                                : selectedUserLabel || t('Select user')
                            }
                          />
                        </SelectTrigger>
                        <SelectContent alignItemWithTrigger={false}>
                          <SelectGroup>
                            {users.map((user) => (
                              <SelectItem key={user.id} value={String(user.id)}>
                                {formatUserOption(user)}
                              </SelectItem>
                            ))}
                            {users.length === 0 && (
                              <SelectItem value='__empty' disabled>
                                {loadingUsers
                                  ? t('Loading users...')
                                  : t('No users found')}
                              </SelectItem>
                            )}
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                    </div>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='payment_method'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Payment Method')}</FormLabel>
                  <FormControl>
                    <Input placeholder='bank_transfer' {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className='grid gap-4 sm:grid-cols-2'>
              <FormField
                control={form.control}
                name='amount'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Recharge Amount')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        step='1'
                        min='1'
                        name={field.name}
                        value={toInputNumberValue(field.value)}
                        onChange={field.onChange}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='money'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Payment Amount')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        step='0.01'
                        min='0'
                        name={field.name}
                        value={toInputNumberValue(field.value)}
                        onChange={field.onChange}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <FormField
              control={form.control}
              name='create_time'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Create Time')}</FormLabel>
                  <FormControl>
                    <Input type='datetime-local' {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='credit_balance'
              render={({ field }) => (
                <FormItem className='flex items-center justify-between gap-3 rounded-lg border p-3'>
                  <div className='flex flex-col gap-1'>
                    <FormLabel>{t('Sync increase balance')}</FormLabel>
                    <p className='text-muted-foreground text-sm'>
                      {t('When enabled, the user balance is credited immediately.')}
                    </p>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />

            <DialogFooter>
              <Button
                type='button'
                variant='outline'
                onClick={() => onOpenChange(false)}
                disabled={submitting}
              >
                {t('Cancel')}
              </Button>
              <Button type='submit' disabled={submitting}>
                {submitting ? t('Creating...') : t('Create')}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
