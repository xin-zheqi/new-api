const { chromium } = require('/tmp/node_modules/playwright')

const baseURL = 'http://host.docker.internal:52093'

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

async function login(page) {
  await page.goto(`${baseURL}/login`, { waitUntil: 'domcontentloaded' })
  await page
    .locator('input:not([type="password"]):not([type="hidden"])')
    .first()
    .fill('root')
  await page.locator('input[type="password"]').first().fill('SmokePass123!')
  const responsePromise = page.waitForResponse(
    (response) =>
      response.request().method() === 'POST' &&
      new URL(response.url()).pathname === '/api/user/login'
  )
  await page
    .getByRole('button', {
      name: /^(继续|登录|登入|Continue|Login|Sign in)$/i,
    })
    .last()
    .click()
  const response = await responsePromise
  const body = await response.json()
  assert(response.ok() && body.success, 'login failed')
  await page.waitForURL((url) => !url.pathname.endsWith('/login'))
  return body.data.access_token
}

async function apiRequest(page, token, path, init = {}) {
  return page.evaluate(
    async ({ init, path, token }) => {
      const response = await fetch(path, {
        ...init,
        headers: {
          Authorization: `Bearer ${token}`,
          'Content-Type': 'application/json',
          ...(init.headers || {}),
        },
      })
      return {
        ok: response.ok,
        status: response.status,
        body: await response.json(),
      }
    },
    { init, path, token }
  )
}

async function setFrontend(page, token, frontend) {
  const response = await apiRequest(page, token, '/api/option/', {
    method: 'PUT',
    body: JSON.stringify({ key: 'theme.frontend', value: frontend }),
  })
  assert(
    response.ok && response.body.success,
    `failed to set frontend ${frontend}`
  )
  const options = await apiRequest(page, token, '/api/option/')
  const savedFrontend = options.body.data?.find(
    (option) => option.key === 'theme.frontend'
  )?.value
  assert(savedFrontend === frontend, `frontend setting remained ${savedFrontend}`)
}

function assertWechatRequests(requests, frontend) {
  assert(
    requests.includes('/api/user/wechat/amount'),
    `${frontend} wallet did not use the WeChat amount endpoint`
  )
  assert(
    requests.includes('/api/user/wechat/pay'),
    `${frontend} wallet did not use the WeChat checkout endpoint`
  )
  assert(
    !requests.includes('/api/user/amount'),
    `${frontend} wallet incorrectly used the Epay amount endpoint`
  )
  assert(
    !requests.includes('/api/user/pay'),
    `${frontend} wallet incorrectly used the Epay checkout endpoint`
  )
}

async function assertDefaultWallet(page, requests) {
  requests.length = 0
  await page.goto(`${baseURL}/console/topup?smoke=${Date.now()}`, {
    waitUntil: 'domcontentloaded',
  })
  const input = page.locator('#topup-amount')
  await input.waitFor({ state: 'visible' })
  await page.waitForFunction(
    () => document.querySelector('#topup-amount')?.value === '5'
  )
  assert(
    Number(await input.inputValue()) === 5,
    `default wallet minimum mismatch: ${await input.inputValue()}`
  )
  await page
    .getByText('最低充值金额：5', { exact: true })
    .waitFor({ state: 'visible' })

  await page.getByRole('button', { name: '微信支付', exact: true }).click()
  const confirmDialog = page.getByRole('alertdialog')
  await confirmDialog.waitFor({ state: 'visible' })
  await confirmDialog
    .getByRole('button', { name: '确认付款', exact: true })
    .click()
  await page
    .getByText('请使用微信扫描二维码完成支付。', { exact: true })
    .waitFor({ state: 'visible' })
  assertWechatRequests(requests, 'default')
  await page.screenshot({
    path: '/work/.docker-test-data/wechat-wallet-default-final.png',
    fullPage: true,
  })
}

async function assertClassicWallet(page, requests) {
  requests.length = 0
  await page.goto(`${baseURL}/console/topup?smoke=${Date.now()}`, {
    waitUntil: 'domcontentloaded',
  })
  await page
    .getByText('最低充值美元数量：5', { exact: true })
    .waitFor({ state: 'visible' })
  const values = await page
    .locator('input')
    .evaluateAll((nodes) => nodes.map((node) => node.value))
  assert(
    values.some((value) => Number(value) === 5),
    `classic wallet minimum mismatch: ${values.join(',')}`
  )

  await page.getByRole('button', { name: '微信支付', exact: true }).click()
  const confirmDialog = page.locator('.semi-modal').filter({
    has: page.getByText('充值确认', { exact: true }),
  })
  await confirmDialog.waitFor({ state: 'visible' })
  await confirmDialog.locator('.semi-modal-footer .semi-button-primary').click()
  await page
    .getByText('请使用微信扫描二维码完成支付', { exact: true })
    .waitFor({ state: 'visible' })
  assertWechatRequests(requests, 'classic')
  await page.screenshot({
    path: '/work/.docker-test-data/wechat-wallet-classic-final.png',
    fullPage: true,
  })
}

async function assertDefaultWechatSettings(page) {
  await page.goto(`${baseURL}/system-settings/billing/payment`, {
    waitUntil: 'domcontentloaded',
  })
  await page.getByRole('tab', { name: '微信支付', exact: true }).click()
  const field = page.getByLabel('最低充值（美元）', { exact: true })
  await field.waitFor({ state: 'visible' })
  assert(
    Number(await field.inputValue()) === 5,
    `default settings minimum mismatch: ${await field.inputValue()}`
  )
}

async function assertClassicWechatSettings(page) {
  await page.goto(`${baseURL}/console/setting?tab=payment`, {
    waitUntil: 'domcontentloaded',
  })
  await page.getByRole('tab', { name: '微信支付设置', exact: true }).click()
  const field = page
    .locator('.semi-form-field:visible')
    .filter({ hasText: '最低充值美元数量' })
    .locator('input:visible')
    .first()
  await field.waitFor({ state: 'visible' })
  assert(
    Number(await field.inputValue()) === 5,
    `classic settings minimum mismatch: ${await field.inputValue()}`
  )
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
  const page = await context.newPage()
  await page.setExtraHTTPHeaders({ 'Cache-Control': 'no-cache' })
  const failures = []
  const requests = []
  let token = ''

  page.on('pageerror', (error) => failures.push(`page error: ${error.message}`))
  page.on('response', (response) => {
    if (response.status() >= 500) {
      failures.push(`HTTP ${response.status()}: ${response.url()}`)
    }
  })
  page.on('request', (request) => {
    if (request.method() !== 'POST') return
    const path = new URL(request.url()).pathname
    if (path.startsWith('/api/user/')) requests.push(path)
  })
  await page.route('**/api/user/wechat/pay', async (route) => {
    const body = route.request().postDataJSON()
    assert(body.amount === 5, `unexpected WeChat checkout amount: ${body.amount}`)
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        message: '',
        data: {
          trade_no: 'W1234567890',
          code_url: 'weixin://wxpay/bizpayurl?pr=browser-smoke',
          amount: '37.25',
          currency: 'CNY',
          expires_at: Math.floor(Date.now() / 1000) + 600,
        },
      }),
    })
  })
  await page.route('**/api/user/wechat/order/**', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        message: '',
        data: { status: 'pending' },
      }),
    })
  )

  try {
    token = await login(page)
    await apiRequest(page, token, '/api/user/self', {
      method: 'PUT',
      body: JSON.stringify({ identity: 'personal' }),
    })

    const info = await apiRequest(page, token, '/api/user/topup/info')
    assert(info.ok && info.body.success, 'failed to load top-up info')
    assert(info.body.data.enable_online_topup === false, 'Epay unexpectedly enabled')
    assert(info.body.data.enable_wechat_topup === true, 'WeChat unexpectedly disabled')
    assert(
      info.body.data.default_payment_method === 'wechat_native_v3',
      `unexpected default method: ${info.body.data.default_payment_method}`
    )
    assert(info.body.data.wechat_min_topup === 5, 'wrong WeChat minimum')
    assert(info.body.data.wechat_unit_price === 7.45, 'wrong WeChat unit price')

    const amount = await apiRequest(page, token, '/api/user/wechat/amount', {
      method: 'POST',
      body: JSON.stringify({ amount: 5 }),
    })
    assert(
      amount.ok && amount.body.success && Number(amount.body.data) === 37.25,
      `unexpected WeChat amount response: ${JSON.stringify(amount.body)}`
    )
    const epay = await apiRequest(page, token, '/api/user/pay', {
      method: 'POST',
      body: JSON.stringify({
        amount: 5,
        payment_method: 'wechat_native_v3',
      }),
    })
    assert(
      epay.ok && epay.body.message === 'error',
      'Epay endpoint accepted the dedicated WeChat method'
    )

    await setFrontend(page, token, 'default')
    await assertDefaultWallet(page, requests)
    await assertDefaultWechatSettings(page)

    await setFrontend(page, token, 'classic')
    await assertClassicWallet(page, requests)
    await assertClassicWechatSettings(page)

    assert(failures.length === 0, failures.join('\n'))
    console.log(
      'PASS: both wallets use official WeChat endpoints and both settings pages show the saved minimum'
    )
  } finally {
    if (token) await setFrontend(page, token, 'classic').catch(() => {})
    await browser.close()
  }
}

main().catch((error) => {
  console.error(error)
  process.exitCode = 1
})
