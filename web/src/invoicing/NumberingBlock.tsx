import { type FormEvent, useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Problems } from '../auth/AuthLayout'
import { formErrors } from '../forms/Form'
import { CaretIcon } from '../layout/icons'
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

  const id = useId()
  const templateErrors = errors.field(`${kind}_id_template`)
  const sequenceErrors = errors.field(`${kind}_sequence_id`)

  // Like Moneybird: labels above the fields, the tag menu beside the
  // template, Save and Cancel once something changed, the example below.
  return (
    <form className="card numbering-card" onSubmit={submit} noValidate>
      <fieldset className="card__fieldset">
        <legend className="numbering-card__title">{title}</legend>
        <div className="numbering-card__body">
          <Problems messages={errors.general} />
          <div className={`numbering-card__field${templateErrors ? ' form-input--invalid' : ''}`}>
            <label htmlFor={`${id}-template`}>{t('invoicing.template')}</label>
            <div className="numbering-card__template">
              <input
                id={`${id}-template`}
                type="text"
                autoComplete="off"
                maxLength={50}
                value={form.template}
                {...(templateErrors ? { 'aria-invalid': true, 'aria-describedby': `${id}-template-error` } : {})}
                onChange={(e) => setForm({ ...form, template: e.target.value })}
              />
              <TagMenu onPick={(tag) => setForm({ ...form, template: `${form.template}{${tag}}` })} />
            </div>
            {templateErrors && <p id={`${id}-template-error`} className="form-input__message form-input__message--error">{templateErrors.join(' ')}</p>}
          </div>
          <div className={`numbering-card__field numbering-card__next${sequenceErrors ? ' form-input--invalid' : ''}`}>
            <label htmlFor={`${id}-next`}>{t('invoicing.nextId')}</label>
            <input
              id={`${id}-next`}
              type="text"
              inputMode="numeric"
              autoComplete="off"
              maxLength={10}
              value={form.sequence}
              aria-describedby={`${id}-next-message`}
              {...(sequenceErrors ? { 'aria-invalid': true } : {})}
              onChange={(e) => setForm({ ...form, sequence: e.target.value })}
            />
            <p id={`${id}-next-message`} className={`form-input__message form-input__message--${sequenceErrors ? 'error' : 'help'}`}>
              {sequenceErrors ? sequenceErrors.join(' ') : t('invoicing.nextIdHelp')}
            </p>
          </div>
          {dirty && (
            <div className="numbering-card__buttons">
              <button className="btn btn--primary btn--small" disabled={update.isPending}>{t('invoicing.save')}</button>
              <button type="button" className="btn btn--secondary btn--small" onClick={() => setForm(saved)}>{t('invoicing.cancel')}</button>
            </div>
          )}
        </div>
        <p className="numbering-card__footer" aria-live="polite">{t('invoicing.example', { value: renderNumber(form.template, form.sequence, new Date(), language) })}</p>
      </fieldset>
    </form>
  )
}

/** The "Insert tag" button and its list of tags. */
function TagMenu({ onPick }: { onPick: (tag: string) => void }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const root = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!open) {
      return
    }
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false)
    const onClick = (e: MouseEvent) => !root.current?.contains(e.target as Node) && setOpen(false)
    document.addEventListener('keydown', onKey)
    document.addEventListener('click', onClick)
    return () => {
      document.removeEventListener('keydown', onKey)
      document.removeEventListener('click', onClick)
    }
  }, [open])
  return (
    <div className="menu" ref={root}>
      <button type="button" className="btn btn--secondary" aria-expanded={open} aria-haspopup="true" onClick={() => setOpen(!open)}>
        {t('invoicing.insertTag')}
        <CaretIcon />
      </button>
      {open && (
        <div className="menu__panel menu__panel--left">
          <ul className="menu__list">
            {numberTags.map((tag) => (
              <li key={tag}>
                <button type="button" className="menu__action" onClick={() => { onPick(tag); setOpen(false) }}>{t(`invoicing.tags.${tag}`)}</button>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}
