import fs from 'node:fs/promises'
import path from 'node:path'

const root = path.resolve('..')
const updates = [
  {
    file: path.join(root, 'web/classic/src/i18n/locales/zh-CN.json'),
    key: 'Before enabling content audit, configure a persistent CRYPTO_SECRET or SESSION_SECRET in the server environment (Docker environment variables or a Compose/Kubernetes Secret). Keep it stable across replicas and restarts; it is not stored on this page.',
    value:
      '启用内容审计前，请在服务器环境中配置持久的 CRYPTO_SECRET 或 SESSION_SECRET（Docker 环境变量，或 Compose/Kubernetes Secret）。请在副本和重启之间保持不变；它不会保存在此页面。',
  },
]

for (const update of updates) {
  const json = JSON.parse(await fs.readFile(update.file, 'utf8'))
  json.translation[update.key] = update.value
  await fs.writeFile(update.file, JSON.stringify(json, null, 2) + '\n', 'utf8')
  console.log(`Updated ${path.relative(root, update.file)}`)
}
