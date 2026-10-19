import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const testDir = path.dirname(fileURLToPath(import.meta.url))
const webDir = path.resolve(testDir, '../../..')
const classicDir = path.join(webDir, 'classic/src/i18n/locales')
const defaultDir = path.join(webDir, 'src/i18n/locales')

function translations(dir, language) {
  const file = path.join(dir, `${language}.json`)
  return JSON.parse(fs.readFileSync(file, 'utf8')).translation
}

test('classic simplified Chinese balance lookup uses Chinese copy, not another locale', () => {
  const active = translations(classicDir, 'zh-CN')
  const reference = translations(classicDir, 'zh')
  const keys = [
    'Balance Query',
    'Private balance lookup',
    'Check your API key balance, usage, and periodic quota in one place.',
    'Please enter an API key',
    'Unable to query balance',
    'Hide API key',
    'Show API key',
    'Querying...',
    'Query',
    'Your key is used only for this request and is not stored by the page.',
    'Unnamed key',
    'No expiration',
    'Periodic quota',
    'Unlimited quota',
    'Total quota',
    'Daily',
    'Weekly',
    'Monthly',
    'Quota period',
    'Period limit',
    'Period used',
    'Period remaining',
    'Current period started',
    'Used quota',
    'Model limits',
    'Enter API Key',
    'Periodic quota usage',
    'Public page for checking API key quota.',
  ]
  for (const key of keys) {
    assert.equal(active[key], reference[key], `zh-CN: ${key}`)
  }
})

test('classic navigation and registration protection labels exist in every runtime locale', () => {
  const keys = [
    '余额查询',
    '公开查询 API Key 的额度与使用情况',
    '订阅设置',
    '订阅套餐设置和用户订阅管理',
    '注册风控',
    '启用同 IP 注册限制',
    '同 IP 每日触发阈值',
    '保存注册风控设置',
    '额度模式',
    '周期额度',
  ]
  for (const language of ['en', 'zh-CN', 'zh-TW', 'fr', 'ja', 'ru', 'vi']) {
    const locale = translations(classicDir, language)
    for (const key of keys) {
      assert.ok(locale[key], `${language}: ${key}`)
      if (language !== 'zh-CN') {
        assert.notEqual(locale[key], key, `${language}: ${key}`)
      }
    }
  }
})

test('default subscription headings and primary actions are not corrupted', () => {
  const keys = [
    'Subscription Settings',
    'Open Subscription',
    'No user subscriptions found',
    'Subscription ID',
    'Subscription opened',
  ]
  for (const language of ['fr', 'ja', 'ru', 'vi']) {
    const locale = translations(defaultDir, language)
    for (const key of keys) {
      assert.ok(locale[key], `${language}: ${key}`)
      assert.doesNotMatch(locale[key], /\?/, `${language}: ${key}`)
    }
  }
})

test('default locale values contain no replacement-question-mark runs', () => {
  for (const language of ['en', 'zh', 'zh-TW', 'fr', 'ja', 'ru', 'vi']) {
    const locale = translations(defaultDir, language)
    for (const [key, value] of Object.entries(locale)) {
      if (typeof value === 'string') {
        assert.doesNotMatch(value, /\?{2,}/, `${language}: ${key}`)
      }
    }
  }
})
