// Integration check for disposable Docker fixtures only; never point at a live site.
import assert from 'node:assert/strict'
const phase = process.env.AUDIT_TEST_PHASE
const hosts = ['newapi-audit-upgrade-mysql', 'newapi-audit-upgrade-postgres', 'newapi-audit-upgrade-sqlite']
for (const host of hosts) {
  const base = 'http://' + host + ':3000'
  const request = async (path, method = 'GET', body, headers = {}) => {
    const response = await fetch(base + path, { method, headers: { 'Content-Type': 'application/json', ...headers }, body: body === undefined ? undefined : JSON.stringify(body) })
    const json = await response.json()
    assert.equal(json.success, true, host + ' ' + path + ': ' + json.message)
    return json.data
  }
  if (phase === 'seed') await request('/api/setup', 'POST', { username: 'upgradetest', password: 'Audit-upgrade-test-20260930!', confirmPassword: 'Audit-upgrade-test-20260930!' })
  const login = await request('/api/user/login', 'POST', { username: 'upgradetest', password: 'Audit-upgrade-test-20260930!' })
  const headers = { Authorization: 'Bearer ' + login.access_token, 'New-Api-User': String(login.user.id) }
  if (phase === 'seed') {
    await request('/api/token/', 'POST', { name: 'Upgrade fixture', remain_quota: 123456, expired_time: -1, group: 'default' }, headers)
    await request('/api/option/', 'PUT', { key: 'AuditUpgradeMarker', value: 'legacy-preserved' }, headers)
    console.log(host + ': legacy user, token and option seeded')
    continue
  }
  assert.equal(phase, 'check')
  const user = await request('/api/user/self', 'GET', undefined, headers)
  assert.equal(user.username, 'upgradetest')
  const tokens = await request('/api/token/', 'GET', undefined, headers)
  const items = Array.isArray(tokens) ? tokens : tokens.items
  assert.equal(items.length, 1)
  assert.equal(items[0].name, 'Upgrade fixture')
  assert.equal(items[0].remain_quota, 123456)
  const options = await request('/api/option/', 'GET', undefined, headers)
  assert.equal(options.find((item) => item.key === 'AuditUpgradeMarker').value, 'legacy-preserved')
  await request('/api/option/', 'PUT', { key: 'ContentAuditSettings', value: '{"enabled":true,"retention_days":7}' }, headers)
  await request('/api/content-audit/policy', 'PUT', { enabled: true, rules: [{ id: 'rule-1', name: 'Fixture', enabled: true, pattern: 'secret', flags: '', mode: 'all', action: 'replace', replacement: 'safe' }] }, headers)
  const policy = await request('/api/content-audit/policy', 'GET', undefined, headers)
  assert.equal(policy.policy.rules[0].replacement, 'safe')
  const preview = await request('/api/content-audit/test', 'POST', { policy: policy.policy, text: 'secret secret' }, headers)
  assert.equal(preview.text, 'safe safe'); assert.equal(preview.result.count, 2)
  const records = await request('/api/content-audit/records', 'GET', undefined, headers)
  assert.equal(records.total, 0, 'preview must not persist a hit')
  const workbook = await fetch(base + '/api/token/usage/export?lang=zh&start_timestamp=100&end_timestamp=200', { headers })
  assert.match(workbook.headers.get('content-type'), /spreadsheetml/)
  assert.equal(workbook.headers.get('cache-control'), 'no-store')
  assert.ok((await workbook.arrayBuffer()).byteLength > 1000)
  await request('/api/option/', 'PUT', { key: 'ContentAuditSettings', value: '{"enabled":false,"retention_days":7}' }, headers)
  const disabled = await fetch(base + '/api/content-audit/policy', { headers })
  assert.equal(disabled.status, 404)
  console.log(host + ': upgrade data preserved; audit, preview, export and disabled gate passed')
}
