import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Problems } from '../auth/AuthLayout'
import { formErrors, SelectInput, TextInput } from '../forms/Form'
import { Block } from '../settings/Block'
import { numberTags, renderNumber } from './numbering'
import { type InvoiceSettings, useUpdateInvoiceSettings } from './queries'

type Props = { administrationId: string; settings: InvoiceSettings; language: string }

/** The invoice and quote number templates, each with a live example. */
export function NumberingBlock({ administrationId, settings, language }: Props) {
  const { t } = useTranslation()
  return (
    <Block id="numbering" title={t('invoicing.parts.numbering')} subtitle={t('invoicing.numberingSubtitle')}>
      <div className="card-stack">
        <NumberCard administrationId={administrationId} kind="invoice" title={t('invoicing.invoiceNumber')} settings={settings} language={language} />
        <NumberCard administrationId={administrationId} kind="estimate" title={t('invoicing.estimateNumber')} settings={settings} language={language} />
      </div>
    </Block>
  )
}

type CardProps = Props & { kind: 'invoice' | 'estimate'; title: string }

function NumberCard({ administrationId, kind, title, settings, language }: CardProps) {
  const { t } = useTranslation()
  const saved = { template: settings[`${kind}_id_template`], sequence: settings[`${kind}_sequence_id`] }
  const [form, setForm] = useState(saved)
  const update = useUpdateInvoiceSettings(administrationId)
  const dirty = form.template !== saved.template || form.sequence !== saved.sequence
  const labels = { [`${kind}_id_template`]: t('invoicing.template'), [`${kind}_sequence_id`]: t('invoicing.nextId') }
  const errors = formErrors(update.error, labels)

  const submit = (e: FormEvent) => {
    e.preventDefault()
    update.mutate({ [`${kind}_id_template`]: form.template, [`${kind}_sequence_id`]: form.sequence })
  }

  return (
    <form className="card" onSubmit={submit} noValidate>
      <fieldset className="card__fieldset">
        <legend className="card__title">{title}</legend>
        <Problems messages={errors.general} />
        <div className="number-fields">
          <TextInput
            label={t('invoicing.template')}
            autoComplete="off"
            maxLength={50}
            value={form.template}
            errors={errors.field(`${kind}_id_template`)}
            onChange={(e) => setForm({ ...form, template: e.target.value })}
          />
          <SelectInput
            label={t('invoicing.insertTag')}
            width="medium"
            value=""
            onChange={(e) => e.target.value && setForm({ ...form, template: `${form.template}{${e.target.value}}` })}
          >
            <option value="">{t('invoicing.insertTag')}</option>
            {numberTags.map((tag) => <option key={tag} value={tag}>{t(`invoicing.tags.${tag}`)}</option>)}
          </SelectInput>
          <TextInput
            label={t('invoicing.nextId')}
            help={t('invoicing.nextIdHelp')}
            width="small"
            inputMode="numeric"
            autoComplete="off"
            maxLength={10}
            value={form.sequence}
            errors={errors.field(`${kind}_sequence_id`)}
            onChange={(e) => setForm({ ...form, sequence: e.target.value })}
          />
        </div>
        <p className="card__footer" aria-live="polite">{t('invoicing.example', { value: renderNumber(form.template, form.sequence, new Date(), language) })}</p>
        {dirty && (
          <div className="form__actions">
            <button className="btn btn--primary btn--small" disabled={update.isPending}>{t('invoicing.save')}</button>
            <button type="button" className="btn btn--secondary btn--small" onClick={() => setForm(saved)}>{t('invoicing.cancel')}</button>
          </div>
        )}
      </fieldset>
    </form>
  )
}
