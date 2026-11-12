import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { CheckboxInput, FormSection, formErrors, SelectInput, TextInput } from '../forms/Form'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { type Identity, useIdentities } from '../settings/queries'
import { type DocumentStyle, useDocumentStyle, useSaveDocumentStyle } from './queries'

/**
 * Adds a document style (/document_styles/new) or changes its name and
 * sender address (/document_styles/:id/edit_general), as Moneybird does
 * before and beside its style editor.
 */
export function DocumentStyleGeneralPage() {
  const { administrationId = '', styleId } = useParams()
  const existing = useDocumentStyle(administrationId, styleId)
  const identities = useIdentities(administrationId)
  if (existing.error instanceof ApiError && existing.error.status < 500) {
    return <NotFoundPage />
  }
  if (!identities.data || (styleId && !existing.data)) {
    return <Loading />
  }
  return <GeneralForm key={existing.data?.id ?? 'new'} administrationId={administrationId} style={existing.data ?? null} identities={identities.data} />
}

type Props = { administrationId: string; style: DocumentStyle | null; identities: Identity[] }

function GeneralForm({ administrationId, style, identities }: Props) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [form, setForm] = useState({
    name: style?.name ?? '',
    identity_id: style?.identity_id ?? identities.find((i) => i.default)?.id ?? '',
    default: style?.default ?? false,
  })
  const save = useSaveDocumentStyle(administrationId, style?.id)
  const isDefault = style?.default ?? false
  const settings = `/${administrationId}/settings/invoices_and_estimates#document-styles`
  const editor = (id: string) => `/${administrationId}/document_styles/${id}/edit/document`
  const back = style ? editor(style.id ?? '') : settings
  const labels = { name: t('documentStyle.name'), identity_id: t('documentStyle.sender') }
  const errors = formErrors(save.error, labels)

  const submit = (e: FormEvent) => {
    e.preventDefault()
    save.mutate(isDefault ? { name: form.name, identity_id: form.identity_id } : form, { onSuccess: (saved) => navigate(editor(saved.id ?? style?.id ?? '')) })
  }

  return (
    <Page title={style ? t('documentStyle.general') : t('documentStyle.new')} back={{ to: back, label: style ? style.name ?? '' : t('documentStyle.back') }}>
      <form className="form-sheet" onSubmit={submit} noValidate>
        <Problems messages={errors.general} />
        <FormSection>
          <TextInput label={labels.name} required maxLength={255} autoComplete="off" value={form.name} errors={errors.field('name')} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          <SelectInput label={labels.identity_id} value={form.identity_id} errors={errors.field('identity_id')} onChange={(e) => setForm({ ...form, identity_id: e.target.value })}>
            {identities.map((i) => <option key={i.id} value={i.id}>{i.company_name}</option>)}
          </SelectInput>
          {!isDefault && <CheckboxInput label={t('documentStyle.default')} checked={form.default} onChange={(e) => setForm({ ...form, default: e.target.checked })} />}
        </FormSection>
        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={save.isPending}>{t('documentStyle.save')}</button>
          <Link className="btn btn--secondary" to={back}>{t('common.cancel')}</Link>
        </div>
      </form>
    </Page>
  )
}
