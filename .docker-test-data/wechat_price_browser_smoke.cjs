const { chromium } = require('/tmp/node_modules/playwright')
const fs = require('fs')

const baseURL = 'http://host.docker.internal:52091'
const outputDir = '/work/.docker-test-data/wechat-price-browser-smoke'

fs.mkdirSync(outputDir, { recursive: true })

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

async function login(page) {
  await page.goto(`${baseURL}/login`, { waitUntil: 'domcontentloaded' })
  await page.locator('input:not([type="password"]):not([type="hidden"])').first().fill('root')
  await page.locator('input[type="password"]').first().fill('SmokePass123!')

  const responsePromise = page.waitForResponse(
    (response) =>
      response.request().method() === 'POST' &&
      new URL(response.url()).pathname === '/api/user/login'
  )
  await page.getByRole('button', { name: /^(继续|登录|登入|Continue|Login|Sign in)$/i }).last().click()
  const response = await responsePromise
  const body = await response.json()
  assert(response.ok() && body.success, 'login failed')
  await page.waitForURL((url) => !url.pathname.endsWith('/login'))
  return body.data.access_token
}

async function setFrontend(page, accessToken, frontend) {
  const result = await page.evaluate(
    async ({ frontend, accessToken }) => {
      const response = await fetch('/api/option/', {
        method: 'PUT',
        headers: {
          Authorization: `Bearer ${accessToken}`,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ key: 'theme.frontend', value: frontend }),
      })
      return response.json()
    },
    { frontend, accessToken }
  )
  assert(result.success, `failed to switch frontend to ${frontend}`)
}

async function main() {
  const browser = await chromium.launch({
    headless: true,
    executablePath: '/ms-playwright/chromium-1187/chrome-linux/chrome',
    args: ['--no-sandbox'],
  })
  const context = await browser.newContext({
    locale: 'zh-CN',
    viewport: { width: 1440, height: 1000 },
  })
  await context.addInitScript(() => localStorage.setItem('i18nextLng', 'zh-CN'))
  const failures = []
  const page = await context.newPage()
  page.on('pageerror', (error) => failures.push(`page error: ${error.message}`))
  page.on('response', (response) => {
    if (response.status() >= 500) failures.push(`HTTP ${response.status()}: ${response.url()}`)
  })
  let accessToken = ''

  try {
    accessToken = await login(page)
    const identityResponse = await page.request.put(`${baseURL}/api/user/self`, {
      headers: { Authorization: `Bearer ${accessToken}` },
      data: { identity: 'personal' },
    })
    const identityBody = await identityResponse.json()
    assert(identityResponse.ok() && identityBody.success, 'failed to set smoke-test identity')

    await setFrontend(page, accessToken, 'classic')
    await page.goto(`${baseURL}/console/setting?tab=payment`, { waitUntil: 'domcontentloaded' })
    const classicTab = page.locator('#semiTabwechat:visible')
    await classicTab.waitFor({ state: 'visible' })
    await classicTab.click()
    await page.waitForFunction(() => document.querySelector('#semiTabwechat')?.getAttribute('aria-selected') === 'true')
    const classicPanel = page.locator('#semiTabPanelwechat:visible')
    const classicLabel = classicPanel.getByText('充值价格（x元/美金）', { exact: true })
    await classicLabel.waitFor({ state: 'visible' })
    const classicInput = classicLabel.locator('xpath=following::input[1]')
    const classicValue = await classicInput.inputValue()
    assert(Number(classicValue) === 7.25, `classic value did not reload as 7.25: ${classicValue}`)
    await classicInput.fill('7.35')
    const classicSaveResponsePromise = page.waitForResponse(
      (response) =>
        response.request().method() === 'PUT' &&
        new URL(response.url()).pathname === '/api/option/wechat'
    )
    await page.getByRole('button', { name: '更新微信支付设置' }).click()
    const classicSaveResponse = await classicSaveResponsePromise
    const classicSaveBody = await classicSaveResponse.json()
    assert(classicSaveResponse.ok() && classicSaveBody.success, 'classic save failed')
    await page.reload({ waitUntil: 'domcontentloaded' })
    await page.locator('#semiTabwechat:visible').click()
    const classicReloadedInput = page
      .locator('#semiTabPanelwechat:visible')
      .getByText('充值价格（x元/美金）', { exact: true })
      .locator('xpath=following::input[1]')
    await classicReloadedInput.waitFor({ state: 'visible' })
    assert(Number(await classicReloadedInput.inputValue()) === 7.35, 'classic saved value was not persisted')
    await page.screenshot({ path: `${outputDir}/classic-wechat-price.png`, fullPage: true })

    await setFrontend(page, accessToken, 'default')
    await page.goto(`${baseURL}/system-settings/billing/payment`, { waitUntil: 'domcontentloaded' })
    await page.getByRole('tab', { name: '微信支付' }).click()
    const defaultLabel = page.getByText('价格（本地货币/美元）', { exact: true }).last()
    await defaultLabel.waitFor({ state: 'visible' })
    const defaultInput = defaultLabel.locator('xpath=following::input[1]')
    assert(Number(await defaultInput.inputValue()) === 7.35, 'default value did not reload as 7.35')
    await defaultInput.fill('7.45')
    const defaultSaveResponsePromise = page.waitForResponse(
      (response) =>
        response.request().method() === 'PUT' &&
        new URL(response.url()).pathname === '/api/option/wechat'
    )
    await page.getByRole('button', { name: '保存所有设置' }).click()
    const defaultSaveResponse = await defaultSaveResponsePromise
    const defaultSaveBody = await defaultSaveResponse.json()
    assert(defaultSaveResponse.ok() && defaultSaveBody.success, 'default save failed')
    await page.reload({ waitUntil: 'domcontentloaded' })
    await page.getByRole('tab', { name: '微信支付' }).click()
    const defaultReloadedInput = page
      .getByText('价格（本地货币/美元）', { exact: true })
      .last()
      .locator('xpath=following::input[1]')
    await defaultReloadedInput.waitFor({ state: 'visible' })
    assert(Number(await defaultReloadedInput.inputValue()) === 7.45, 'default saved value was not persisted')
    await page.screenshot({ path: `${outputDir}/default-wechat-price.png`, fullPage: true })

    assert(failures.length === 0, failures.join('\n'))
    console.log('PASS: classic and default WeChat price fields save and reload independently')
  } finally {
    if (accessToken) {
      await setFrontend(page, accessToken, 'classic').catch(() => {})
    }
    await browser.close()
  }
}

main().catch((error) => {
  console.error(error)
  process.exitCode = 1
})
