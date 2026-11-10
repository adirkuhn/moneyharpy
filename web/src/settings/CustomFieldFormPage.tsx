import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Problems } from '../auth/AuthLayout'
import { FormSection, formErrors, SelectInput, TextInput } from '../forms/Form'
import { Page } from '../layout/Page'
import { type CustomFieldSource, useCreateCustomField } from './queries'

const sources: CustomFieldSource[] = ['identity', 'contact', 'sales_invoice', 'estimate']

/**
 * returnPath keeps a requested return path only when it stays inside the
 * administration, so the link cannot send people to another site.
 */
export function returnPath(administrationId: string, requested: string | null): string {
  const inside = `/${administrationId}/`
  return requested?.startsWith(inside) && !requested.startsWith('//') ? requested : `${inside}settings/company`
}

/** Adds a custom field, then goes back to the form that asked for it. */
export function CustomFieldFormPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { administrationId = '' } = useParams()
  const [params] = useSearchParams()
  const requestedSource = params.get('source') as CustomFieldSource | null
  const [name, setName] = useState('')
  const [source, setSource] = useState<CustomFieldSource>(requestedSource && sources.includes(requestedSource) ? requestedSource : 'identity')
  const create = useCreateCustomField(administrationId)
  const back = returnPath(administrationId, params.get('return_to'))
  const labels = { name: t('customField.name'), source: t('customField.source') }
  const errors = formErrors(create.error, labels)

  const submit = (e: FormEvent) => {
    e.preventDefault()
    create.mutate({ name, source }, { onSuccess: () => navigate(back) })
  }

  return (
    <Page title={t('customField.title')} back={{ to: back, label: t('customField.back') }}>
      <form className="form-sheet" onSubmit={submit} noValidate>
        <Problems messages={errors.general} />
        <FormSection>
          <TextInput label={labels.name} required maxLength={255} autoComplete="off" value={name} errors={errors.field('name')} onChange={(e) => setName(e.target.value)} />
          <SelectInput label={labels.source} width="medium" value={source} errors={errors.field('source')} onChange={(e) => setSource(e.target.value as CustomFieldSource)}>
            {sources.map((s) => <option key={s} value={s}>{t(`customField.sources.${s}`)}</option>)}
          </SelectInput>
        </FormSection>
        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={create.isPending}>{t('settings.save')}</button>
          <Link className="btn btn--secondary" to={back}>{t('common.cancel')}</Link>
        </div>
      </form>
    </Page>
  )
}
