import { type FullConfig, request } from '@playwright/test'

/**
 * Runs once, before any test registers, so this account becomes the
 * instance administrator; it then points the instance at the stack's mail
 * and storage services. The specs run in parallel after it, each with users
 * and administrations of their own.
 */
export default async function globalSetup(config: FullConfig) {
  const baseURL = config.projects[0]?.use.baseURL
  const ip = () => Math.floor(Math.random() * 255)
  const api = await request.newContext({ baseURL, extraHTTPHeaders: { 'X-Forwarded-For': `10.${ip()}.${ip()}.${ip()}` } })
  const admin = { email: 'admin@example.com', name: 'Instance Admin', password: 'correct horse battery' }
  const signedIn = await api.post('/api/auth/login', { data: { email: admin.email, password: admin.password } })
  if (!signedIn.ok()) {
    const registered = await api.post('/api/auth/register', { data: admin })
    if (registered.status() !== 201) {
      throw new Error(`register the instance admin: ${registered.status()} ${await registered.text()}`)
    }
  }
  const saved = await api.patch('/api/instance/settings', {
    data: {
      smtp_enabled: true, smtp_host: 'mailpit', smtp_port: 1025, smtp_from: 'books@example.com', smtp_security: 'none',
      storage_enabled: true, storage_endpoint: 'http://storage:8333', storage_region: 'us-east-1', storage_bucket: 'moneyharpy',
      storage_access_key: 's3-test', storage_secret_key: 's3-test-secret', storage_path_style: true,
      webhooks_allow_local: true,
    },
  })
  if (saved.status() !== 200) {
    throw new Error(`instance settings: ${saved.status()} ${await saved.text()}`)
  }
  // Creates the bucket on first run.
  const storage = await api.post('/api/instance/settings/test_storage')
  if (storage.status() !== 204) {
    throw new Error(`test storage: ${storage.status()} ${await storage.text()}`)
  }
  await api.dispose()
}
