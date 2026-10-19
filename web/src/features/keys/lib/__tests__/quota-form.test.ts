/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import type { TFunction } from 'i18next'
import { describe, expect, test } from 'vitest'

import { apiKeySchema } from '../../types'
import {
  getApiKeyFormDefaultValues,
  getApiKeyFormSchema,
  transformApiKeyToFormDefaults,
  transformFormDataToPayload,
} from '../api-key-form'

const t = ((key: string) => key) as TFunction

const apiKey = apiKeySchema.parse({
  id: 1,
  name: 'total quota',
  key: 'sk-test',
  status: 1,
  remain_quota: 1200000,
  used_quota: 0,
  unlimited_quota: false,
  quota_period: '',
  quota_period_limit: 0,
  expired_time: -1,
  created_time: 1,
  accessed_time: 0,
  group: 'default',
  model_limits_enabled: false,
  model_limits: '',
  allow_ips: '',
})

describe('API key quota mode form mapping', () => {
  test('keeps the total quota option visible and submits the API empty value', () => {
    const values = {
      ...getApiKeyFormDefaultValues(false),
      name: 'total quota',
      unlimited_quota: false,
      remain_quota_dollars: 12,
    }

    expect(values.quota_period).toBe('total')
    expect(getApiKeyFormSchema(t).safeParse(values).success).toBe(true)

    const payload = transformFormDataToPayload(values)
    expect(payload.quota_period).toBe('')
    expect(payload.unlimited_quota).toBe(false)
    expect(payload.remain_quota).toBeGreaterThan(0)
  })

  test('maps an existing total quota from the API to the explicit UI option', () => {
    const values = transformApiKeyToFormDefaults(apiKey)

    expect(values.quota_period).toBe('total')
    expect(values.unlimited_quota).toBe(false)
    expect(values.remain_quota_dollars).toBeGreaterThan(0)
  })

  test('keeps periodic quota payloads unchanged', () => {
    const values = {
      ...getApiKeyFormDefaultValues(false),
      name: 'daily quota',
      quota_period: 'day' as const,
      quota_period_limit_dollars: 5,
    }

    const payload = transformFormDataToPayload(values)
    expect(payload.quota_period).toBe('day')
    expect(payload.quota_period_limit).toBeGreaterThan(0)
    expect(payload.remain_quota).toBe(0)
    expect(payload.unlimited_quota).toBe(true)
  })
})
