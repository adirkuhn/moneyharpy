import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { FormSection, formErrors, SelectInput, TextInput } from '../forms/Form'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { type CustomField, type CustomFieldSource, useCreateCustomField, useCustomField, useDeleteCustomField, useUpdateCustomField } from './queries'

const sources: CustomFieldSource[] = ['identity', 'contact', 'sales_invoice', 'estimate']

/**
 * returnPath keeps a requested return path only when it stays inside the
 * administration, so the link cannot send people to another site.
 */
export function returnPath(administrationId: string, requested: string | null, fallback = 'company'): string {
  const inside = `/${administrationId}/`
  return requested?.startsWith(inside) && !requested.startsWith('//') ? requested : `${inside}settings/${fallback}`
}

/** Adds a custom field (/custom_fields/new) or edits one, then goes back. */
export function CustomFieldFormPage() {
  const { administrationId = '', fieldId } = useParams()
  const existing = useCustomField(administrationId, fieldId)
  if (existing.error instanceof ApiError && existing.error.status < 500) {
    return <NotFoundPage />
  }
  if (fieldId && !existing.data) {
    return <Loading />
  }
  return <CustomFieldForm key={existing.data?.id ?? 'new'} administrationId={administrationId} field={existing.data ?? null} />
}

function CustomFieldForm({ administrationId, field }: { administrationId: string; field: CustomField | null }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const requestedSource = params.get('source') as CustomFieldSource | null
  const [name, setName] = useState(field?.name ?? '')
  const [source, setSource] = useState<CustomFieldSource>(field?.source ?? (requestedSource && sources.includes(requestedSource) ? requestedSource : 'identity'))
  const create = useCreateCustomField(administrationId)
  const update = useUpdateCustomField(administrationId, field?.id ?? '')
  const remove = useDeleteCustomField(administrationId)
  const back = returnPath(administrationId, params.get('return_to'), field ? 'invoices_and_estimates' : 'company')
  const labels = { name: t('customField.name'), source: t('customField.source') }
  const errors = formErrors(create.error ?? update.error ?? remove.error, labels)

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (field) {
      update.mutate(name, { onSuccess: () => navigate(back) })
    } else {
      create.mutate({ name, source }, { onSuccess: () => navigate(back) })
    }
  }

  return (
    <Page
      title={field ? t('customField.edit') : t('customField.title')}
      back={{ to: back, label: t('customField.back') }}
      actions={field ? <ConfirmDelete label={t('customField.delete')} confirmLabel={t('customField.confirmDelete')} disabled={remove.isPending} onConfirm={() => remove.mutate(field.id, { onSuccess: () => navigate(back) })} /> : undefined}
    >
      <form className="form-sheet" onSubmit={submit} noValidate>
        <Problems messages={errors.general} />
        <FormSection>
          <TextInput label={labels.name} required maxLength={255} autoComplete="off" value={name} errors={errors.field('name')} onChange={(e) => setName(e.target.value)} />
          <SelectInput
            label={labels.source}
            width="medium"
            value={source}
            disabled={field !== null}
            help={field ? t('customField.sourceFixed') : undefined}
            errors={errors.field('source')}
            onChange={(e) => setSource(e.target.value as CustomFieldSource)}
          >
            {sources.map((s) => <option key={s} value={s}>{t(`customField.sources.${s}`)}</option>)}
          </SelectInput>
        </FormSection>
        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={create.isPending || update.isPending}>{t('settings.save')}</button>
          <Link className="btn btn--secondary" to={back}>{t('common.cancel')}</Link>
        </div>
      </form>
    </Page>
  )
}
