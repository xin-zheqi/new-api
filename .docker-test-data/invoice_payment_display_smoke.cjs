const { chromium } = require('/tmp/node_modules/playwright')

const baseURL = process.env.BASE_URL
const username = process.env.TEST_USERNAME
const password = process.env.TEST_PASSWORD

if (!baseURL || !username || !password) {
  throw new Error('BASE_URL, TEST_USERNAME, and TEST_PASSWORD are required')
}

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

async function login(page) {
  await page.goto(`${baseURL}/login`, { waitUntil: 'domcontentloaded' })
  await page
    .locator(
      'input:not([type="password"]):not([type="hidden"]):not([type="checkbox"])'
    )
    .first()
    .fill(username)
  await page.locator('input[type="password"]').first().fill(password)
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

async function setFrontend(page, token, frontend) {
  const result = await page.evaluate(
    async ({ accessToken, value }) => {
      const response = await fetch('/api/option/', {
        method: 'PUT',
        credentials: 'same-origin',
        headers: {
          Authorization: `Bearer ${accessToken}`,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ key: 'theme.frontend', value }),
      })
      return { status: response.status, body: await response.json() }
    },
    { accessToken: token, value: frontend }
  )
  assert(
    result.status === 200 && result.body.success,
    `failed to switch frontend to ${frontend}`
  )
}

async function verifyInvoiceCenter(page, frontend, path) {
  await page.goto(`${baseURL}${path}?smoke=${Date.now()}`, {
    waitUntil: 'domcontentloaded',
  })
  await page.getByText('微信支付', { exact: true }).first().waitFor({
    state: 'visible',
    timeout: 15000,
  })
  const bodyText = await page.locator('body').innerText()
  assert(
    !bodyText.includes('wechat_native_v3'),
    `${frontend} frontend exposed the internal payment identifier`
  )
  assert(bodyText.includes('12.34'), `${frontend} frontend lost the invoice amount`)
  await page.screenshot({
    path: `/work/.docker-test-data/invoice-payment-${frontend}.png`,
    fullPage: true,
  })
}

async function main() {
  const browser = await chromium.launch({
    headless: true,
    executablePath: '/ms-playwright/chromium-1187/chrome-linux/chrome',
    args: ['--no-sandbox'],
  })
  const failures = []
  try {
    const context = await browser.newContext({
      locale: 'zh-CN',
      viewport: { width: 1440, height: 1000 },
    })
    await context.addInitScript(() => {
      localStorage.setItem('i18nextLng', 'zh-CN')
    })
    const page = await context.newPage()
    page.on('pageerror', (error) => failures.push(`page error: ${error.message}`))
    page.on('response', (response) => {
      if (response.status() >= 500) {
        failures.push(`HTTP ${response.status()}: ${response.url()}`)
      }
    })

    const token = await login(page)
    await setFrontend(page, token, 'default')
    await verifyInvoiceCenter(page, 'default', '/invoice-center/')
    await setFrontend(page, token, 'classic')
    await verifyInvoiceCenter(page, 'classic', '/console/invoice')
    assert(failures.length === 0, failures.join('\n'))
    await context.close()
    console.log('PASS: default and classic invoice centers display 微信支付')
  } finally {
    await browser.close()
  }
}

main().catch((error) => {
  console.error(error)
  process.exitCode = 1
})
