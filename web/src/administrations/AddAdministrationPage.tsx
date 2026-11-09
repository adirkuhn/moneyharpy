import { type FormEvent, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { errorMessages, type Language } from '../api/client'
import { Field, Problems } from '../auth/AuthLayout'
import { Page } from '../layout/Page'
import { useCreateAdministration } from './queries'

const labels = { name: 'Company name', language: 'Language', currency: 'Currency', country: 'Country', time_zone: 'Time zone' }

export function AddAdministrationPage() {
  const [form, setForm] = useState({
    name: '',
    language: 'en' as Language,
    currency: 'EUR',
    country: 'NL',
    time_zone: 'Europe/Amsterdam',
  })
  const navigate = useNavigate()
  const create = useCreateAdministration()

  const submit = (e: FormEvent) => {
    e.preventDefault()
    create.mutate(form, { onSuccess: (a) => navigate(a.uri) })
  }
  const set = (key: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) =>
    setForm({ ...form, [key]: e.target.value })

  return (
    <Page title="Add administration">
      <form className="form" onSubmit={submit}>
        <Problems messages={errorMessages(create.error, labels)} />
        <Field label="Company name" required maxLength={255} value={form.name} onChange={set('name')} />
        <label className="field">
          <span>Language</span>
          <select value={form.language} onChange={set('language')}>
            <option value="en">English</option>
            <option value="nl">Dutch</option>
            <option value="nl-be">Dutch (Belgium)</option>
          </select>
        </label>
        <Field label="Currency" required maxLength={3} value={form.currency} onChange={set('currency')} />
        <Field label="Country" required maxLength={2} value={form.country} onChange={set('country')} />
        <Field label="Time zone" required value={form.time_zone} onChange={set('time_zone')} />
        <button className="btn btn--primary" disabled={create.isPending}>Add administration</button>
      </form>
    </Page>
  )
}
