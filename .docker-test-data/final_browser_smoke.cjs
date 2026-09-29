const { chromium } = require('/tmp/node_modules/playwright')
const fs = require('fs')

const baseURL = process.env.BASE_URL || 'http://host.docker.internal:52090'
const username = process.env.TEST_USERNAME
const password = process.env.TEST_PASSWORD
const outputDir = '/work/.docker-test-data/final-browser-smoke'

if (!username || !password) {
  throw new Error('TEST_USERNAME and TEST_PASSWORD are required')
}

fs.mkdirSync(outputDir, { recursive: true })

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

async function login(page, frontend) {
  await page.goto(`${baseURL}/login`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(1000)
  await page.screenshot({
    path: `${outputDir}/${frontend}-login.png`,
    fullPage: true,
  })

  const passwordInput = page.locator('input[type="password"]').first()
  const usernameInput = page
    .locator('input:not([type="password"]):not([type="hidden"]):not([type="checkbox"])')
    .first()
  await usernameInput.fill(username)
  await passwordInput.fill(password)

  const loginResponsePromise = page.waitForResponse(
    (response) =>
      response.request().method() === 'POST' &&
      new URL(response.url()).pathname === '/api/user/login'
  )
  const loginButton = page
    .getByRole('button', { name: /^(继续|登录|登入|Continue|Login|Sign in)$/i })
    .last()
  await loginButton.click()
  const loginResponse = await loginResponsePromise
  const loginResponseText = await loginResponse.text()
  let loginBody = null
  try {
    loginBody = JSON.parse(loginResponseText)
  } catch {
    throw new Error(
      `${frontend}: login returned HTTP ${loginResponse.status()} without JSON`
    )
  }
  assert(
    loginResponse.ok() && loginBody.success && loginBody.data?.access_token,
    `${frontend}: login request failed`
  )
  await page.waitForURL((url) => !url.pathname.endsWith('/login'), {
    timeout: 15000,
  })

  const selfResponse = await page.request.get(`${baseURL}/api/user/self`, {
    headers: { Authorization: `Bearer ${loginBody.data.access_token}` },
  })
  assert(selfResponse.ok(), `${frontend}: login did not create a usable session`)
  return loginBody.data
}

async function setFrontend(page, auth, value) {
  const result = await page.evaluate(async ({ frontend, accessToken }) => {
    const response = await fetch('/api/option/', {
      method: 'PUT',
      credentials: 'same-origin',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${accessToken}`,
      },
      body: JSON.stringify({ key: 'theme.frontend', value: frontend }),
    })
    return { status: response.status, body: await response.json() }
  }, { frontend: value, accessToken: auth.access_token })
  assert(
    result.status === 200 && result.body.success,
    `failed to switch frontend to ${value}: ${JSON.stringify(result)}`
  )
}

async function fieldByText(scope, label) {
  const text = scope.getByText(label, { exact: true }).last()
  await text.waitFor({ state: 'visible', timeout: 10000 })
  const input = text.locator('xpath=following::input[1]')
  await input.waitFor({ state: 'visible', timeout: 10000 })
  return input
}

async function verifyManualTopupDialog(page, frontend) {
  const labels =
    frontend === 'classic'
      ? {
          paymentAmount: '支付金额',
          paymentCurrency: '支付币种',
          invoiceAmount: '开票金额',
          invoiceCurrency: '开票币种',
        }
      : {
          paymentAmount: '支付金额',
          paymentCurrency: '支付币种',
          invoiceAmount: '可开票金额',
          invoiceCurrency: '开票币种',
        }

  const createButton = page
    .getByRole('button', { name: /^(创建充值记录|Create recharge record)$/i })
    .first()
  await createButton.waitFor({ state: 'visible', timeout: 15000 })
  await createButton.click()
  const dialog = page.locator('[role="dialog"]:visible').last()
  await dialog.waitFor({ state: 'visible', timeout: 10000 })
  await page.screenshot({
    path: `${outputDir}/${frontend}-manual-topup-open.png`,
    fullPage: true,
  })

  const paymentAmount = await fieldByText(dialog, labels.paymentAmount)
  const paymentCurrency = await fieldByText(dialog, labels.paymentCurrency)
  const invoiceAmount = await fieldByText(dialog, labels.invoiceAmount)
  const invoiceCurrency = await fieldByText(dialog, labels.invoiceCurrency)

  assert((await paymentCurrency.inputValue()) === 'CNY', `${frontend}: payment currency default is not CNY`)
  assert((await invoiceCurrency.inputValue()) === 'CNY', `${frontend}: invoice currency default is not CNY`)

  await paymentAmount.fill('1.234567')
  await paymentCurrency.fill('kwd')
  await invoiceAmount.fill('1.111111')
  await invoiceCurrency.fill('kwd')

  assert((await paymentAmount.inputValue()) === '1.234567', `${frontend}: payment amount lost precision`)
  assert((await paymentCurrency.inputValue()) === 'KWD', `${frontend}: payment currency was not normalized`)
  assert((await invoiceAmount.inputValue()) === '1.111111', `${frontend}: invoice amount lost precision`)
  assert((await invoiceCurrency.inputValue()) === 'KWD', `${frontend}: invoice currency was not normalized`)

  for (const [left, right, name] of [
    [paymentAmount, paymentCurrency, 'payment'],
    [invoiceAmount, invoiceCurrency, 'invoice'],
  ]) {
    const leftBox = await left.boundingBox()
    const rightBox = await right.boundingBox()
    assert(leftBox && rightBox, `${frontend}: ${name} fields have no layout box`)
    assert(Math.abs(leftBox.y - rightBox.y) < 12, `${frontend}: ${name} fields are not aligned`)
    assert(leftBox.x + leftBox.width <= rightBox.x, `${frontend}: ${name} fields overlap`)
  }

  await page.screenshot({
    path: `${outputDir}/${frontend}-manual-topup.png`,
    fullPage: true,
  })
}

async function newContext(browser) {
  const context = await browser.newContext({
    locale: 'zh-CN',
    viewport: { width: 1440, height: 1000 },
    deviceScaleFactor: 1,
  })
  await context.addInitScript(() => {
    localStorage.setItem('i18nextLng', 'zh-CN')
  })
  return context
}

async function main() {
  const browser = await chromium.launch({
    headless: true,
    executablePath: '/ms-playwright/chromium-1187/chrome-linux/chrome',
    args: ['--no-sandbox'],
  })
  const failures = []

  try {
    const context = await newContext(browser)
    let page = await context.newPage()
    page.on('pageerror', (error) => failures.push(`classic page error: ${error.message}`))
    page.on('response', (response) => {
      if (response.status() >= 500) failures.push(`classic HTTP ${response.status()}: ${response.url()}`)
    })
    const auth = await login(page, 'classic')
    await page.goto(`${baseURL}/console/topup?show_history=true`, {
      waitUntil: 'domcontentloaded',
    })
    await page.waitForTimeout(1500)
    await page.screenshot({ path: `${outputDir}/classic-wallet.png`, fullPage: true })
    await verifyManualTopupDialog(page, 'classic')
    await setFrontend(page, auth, 'default')

    const anonymousContext = await newContext(browser)
    const loginPage = await anonymousContext.newPage()
    await loginPage.goto(`${baseURL}/login`, { waitUntil: 'domcontentloaded' })
    await loginPage.waitForTimeout(1000)
    await loginPage.screenshot({
      path: `${outputDir}/default-login.png`,
      fullPage: true,
    })
    await anonymousContext.close()

    page = await context.newPage()
    page.on('pageerror', (error) => failures.push(`default page error: ${error.message}`))
    page.on('response', (response) => {
      if (response.status() >= 500) failures.push(`default HTTP ${response.status()}: ${response.url()}`)
    })
    await page.goto(`${baseURL}/wallet?show_history=true`, {
      waitUntil: 'domcontentloaded',
    })
    await page.waitForTimeout(1500)
    await page.screenshot({ path: `${outputDir}/default-wallet.png`, fullPage: true })
    await verifyManualTopupDialog(page, 'default')
    await setFrontend(page, auth, 'classic')
    await context.close()

    assert(failures.length === 0, failures.join('\n'))
    console.log('PASS: classic and default frontend browser smoke tests')
    console.log(`Screenshots: ${outputDir}`)
  } finally {
    await browser.close()
  }
}

main().catch((error) => {
  console.error(error)
  process.exitCode = 1
})
