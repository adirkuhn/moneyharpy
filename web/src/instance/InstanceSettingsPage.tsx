import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { type FormEvent, useEffect, useState } from 'react'
import { api, errorMessages, type InstanceSettings, unwrap, type UpdateInstanceSettings } from '../api/client'
import { Field, Problems } from '../auth/AuthLayout'
import { useCurrentUser } from '../auth/queries'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'

const labels = { google_client_id: 'Google client ID', google_client_secret: 'Google client secret' }

export function InstanceSettingsPage() {
  const me = useCurrentUser()
  if (!me.data?.instance_admin) {
    return <NotFoundPage />
  }
  return <SettingsForm />
}

function SettingsForm() {
  const client = useQueryClient()
  const settings = useQuery({ queryKey: ['instance-settings'], queryFn: () => unwrap(api.GET('/api/instance/settings')) })
  const [form, setForm] = useState<InstanceSettings | null>(null)
  const [secret, setSecret] = useState('')

  useEffect(() => {
    if (settings.data) {
      setForm(settings.data)
    }
  }, [settings.data])

  const save = useMutation({
    mutationFn: (body: UpdateInstanceSettings) => unwrap(api.PATCH('/api/instance/settings', { body })),
    onSuccess: (saved) => {
      setSecret('')
      client.setQueryData(['instance-settings'], saved)
      void client.invalidateQueries({ queryKey: ['providers'] })
    },
  })

  if (!form) {
    return null
  }

  const submit = (e: FormEvent) => {
    e.preventDefault()
    save.mutate({
      registration_open: form.registration_open,
      google_enabled: form.google_enabled,
      google_client_id: form.google_client_id,
      ...(secret ? { google_client_secret: secret } : {}),
    })
  }

  return (
    <Page title="Instance settings">
      <form className="form" onSubmit={submit}>
        <Problems messages={errorMessages(save.error, labels)} />
        {save.isSuccess && <p role="status" className="saved">Settings saved</p>}
        <label className="check">
          <input type="checkbox" checked={form.registration_open} onChange={(e) => setForm({ ...form, registration_open: e.target.checked })} />
          <span>Allow new users to register</span>
        </label>
        <h2>Google login</h2>
        <label className="check">
          <input type="checkbox" checked={form.google_enabled} onChange={(e) => setForm({ ...form, google_enabled: e.target.checked })} />
          <span>Let users continue with Google</span>
        </label>
        <Field label="Google client ID" value={form.google_client_id} onChange={(e) => setForm({ ...form, google_client_id: e.target.value })} />
        <Field
          label="Google client secret"
          type="password"
          autoComplete="off"
          placeholder={form.google_client_secret_set ? 'Stored. Type to replace it.' : ''}
          value={secret}
          onChange={(e) => setSecret(e.target.value)}
        />
        <button className="btn btn--primary" disabled={save.isPending}>Save settings</button>
      </form>
    </Page>
  )
}
