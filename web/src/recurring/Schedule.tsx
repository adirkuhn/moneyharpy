import { type FormEvent, useId } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { Problems } from '../auth/AuthLayout'
import { todayDay } from '../forms/calendar'
import { DatePicker } from '../forms/DatePicker'
import { type FormErrors, FormRow } from '../forms/Form'
import { invoiceTitle, type SalesInvoice } from '../invoices/queries'
import { frequencyText, frequencyTypes, type FrequencyType, type RecurringChanges, type RecurringInvoice } from './queries'
import { countUntil, frequencyOfPreset, occurrence, type Preset, presetOf, presets, upcoming } from './schedule'

/** The schedule as its form holds it, after Moneybird's: start date, frequency, how many more and sending. */
export type ScheduleDraft = {
  invoiceDate: string
  preset: Preset
  /** The "Every n units" of the custom choice. */
  frequency: string
  frequencyType: FrequencyType
  hasCount: boolean
  count: string
  autoSend: boolean
  mergeable: boolean
}

/** The form of a saved recurring invoice's schedule, or of a new one's starting today. */
export function draftOf(r: RecurringInvoice | null, today: string): ScheduleDraft {
  const frequency = r?.frequency ?? 1
  const frequencyType = r?.frequency_type ?? 'month'
  const count = r ? countUntil(r) : null
  return {
    invoiceDate: r?.invoice_date ?? today, preset: r ? presetOf(frequency, frequencyType) : '1:month', frequency: String(frequency), frequencyType,
    hasCount: count !== null, count: count === null ? '' : String(count), autoSend: r?.auto_send ?? false, mergeable: r?.mergeable ?? false,
  }
}

/** The schedule the draft describes, as the timeline of a new recurring invoice shows it. */
export function scheduleOfDraft(d: ScheduleDraft): Pick<RecurringInvoice, 'invoice_date' | 'frequency' | 'frequency_type' | 'last_date'> {
  const { frequency, frequencyType } = frequencyOfPreset(d.preset) ?? { frequency: Number(d.frequency) || 1, frequencyType: d.frequencyType }
  const count = Number(d.count)
  const last = d.hasCount && count > 0 ? occurrence(d.invoiceDate, frequencyType, frequency, count - 1) : null
  return { invoice_date: d.invoiceDate, frequency, frequency_type: frequencyType, last_date: last }
}

/**
 * The changes the draft makes. "Add another n times" counts from now on, so
 * the server, which counts every invoice, hears the ones made so far as well.
 * The start date goes along when it changed, as a new one restarts the schedule.
 */
export function scheduleChanges(d: ScheduleDraft, r: RecurringInvoice | null): RecurringChanges {
  const preset = frequencyOfPreset(d.preset)
  // What was typed goes as it is when it is no number, so the server names the problem.
  const typed = d.frequency.trim()
  return {
    ...(d.invoiceDate !== r?.invoice_date ? { invoice_date: d.invoiceDate } : {}),
    frequency: preset ? preset.frequency : Number(typed) || typed,
    frequency_type: preset ? preset.frequencyType : d.frequencyType,
    has_desired_count: d.hasCount,
    ...(d.hasCount ? { desired_count: (r?.invoices_created ?? 0) + (Number(d.count) || 0) } : {}),
    auto_send: d.autoSend,
    mergeable: d.autoSend && d.mergeable,
  }
}

/** What the server's answers about the schedule are called on the form. */
export function scheduleLabels(t: (k: string) => string): Record<string, string> {
  return {
    invoice_date: t('recurring.startDate'), frequency: t('recurring.frequency'), frequency_type: t('recurring.frequency'),
    desired_count: t('recurring.addAnother'), has_desired_count: t('recurring.addAnother'), last_date: t('recurring.addAnother'),
    auto_send: t('recurring.autoSendText'), mergeable: t('recurring.mergeable'),
  }
}

type FormProps = {
  value: ScheduleDraft
  onChange: (change: Partial<ScheduleDraft>) => void
  /** Continue on a new recurring invoice; Save and Cancel on a saved one. */
  submitLabel: string
  onSubmit: () => void
  onCancel?: () => void
  pending?: boolean
  errors: FormErrors
}

/**
 * Moneybird's schedule of a recurring invoice: the start date, the
 * frequency as four common choices or "Every n units", "Add another n
 * times", and sending each invoice automatically.
 */
export function ScheduleForm({ value, onChange, submitLabel, onSubmit, onCancel, pending = false, errors }: FormProps) {
  const { t } = useTranslation()
  const id = useId()
  const submit = (e: FormEvent) => {
    e.preventDefault()
    onSubmit()
  }
  // A choice shows itself in "Every n units" too, as Moneybird's fields follow it.
  const choosePreset = (p: Exclude<Preset, 'custom'>) => {
    const f = frequencyOfPreset(p)
    onChange({ preset: p, ...(f ? { frequency: String(f.frequency), frequencyType: f.frequencyType } : {}) })
  }
  const frequencyErrors = [...(errors.field('frequency') ?? []), ...(errors.field('frequency_type') ?? [])]
  const countErrors = [...(errors.field('desired_count') ?? []), ...(errors.field('last_date') ?? [])]
  return (
    <form className="recurring-schedule__form" onSubmit={submit} noValidate aria-label={t('recurring.scheduleForm')}>
      <Problems messages={errors.general} />
      <FormRow label={t('recurring.startDate')} width="medium" errors={errors.field('invoice_date')}>
        {({ labelId: _, ...control }) => (
          <DatePicker {...control} value={value.invoiceDate} min={todayDay()} onChange={(d) => onChange({ invoiceDate: d })} />
        )}
      </FormRow>
      <hr />
      <div className={`form-input form-input--long${frequencyErrors.length ? ' form-input--invalid' : ''}`}>
        <span className="form-input__label" id={`${id}-frequency`}>{t('recurring.frequency')}</span>
        <div className="form-input__body">
          <div className="recurring-schedule__presets" role="radiogroup" aria-labelledby={`${id}-frequency`}>
            {presets.map((p) => (
              <label key={p} className="radio-option">
                <input type="radio" name={`${id}-preset`} value={p} checked={value.preset === p} onChange={() => choosePreset(p)} />
                <span>{t(`recurring.presets.${p.replace(':', '_')}`)}</span>
              </label>
            ))}
            <label className="radio-option recurring-schedule__custom">
              {/* Named by its word alone: the fields after it have names of their own. */}
              <input type="radio" name={`${id}-preset`} value="custom" aria-label={t('recurring.presets.custom')} checked={value.preset === 'custom'} onChange={() => onChange({ preset: 'custom' })} />
              <span>{t('recurring.presets.custom')}</span>
              <input
                className="recurring-schedule__tiny"
                type="text"
                inputMode="numeric"
                maxLength={4}
                aria-label={t('recurring.interval')}
                value={value.frequency}
                onFocus={() => onChange({ preset: 'custom' })}
                onChange={(e) => onChange({ preset: 'custom', frequency: e.target.value })}
              />
              <select
                aria-label={t('recurring.unit')}
                value={value.frequencyType}
                onFocus={() => onChange({ preset: 'custom' })}
                onChange={(e) => onChange({ preset: 'custom', frequencyType: e.target.value as FrequencyType })}
              >
                {frequencyTypes.map((f) => <option key={f} value={f}>{t(`recurring.units.${f}`)}</option>)}
              </select>
            </label>
          </div>
          {frequencyErrors.length > 0 && <p className="form-input__message form-input__message--error">{frequencyErrors.join(' ')}</p>}
        </div>
      </div>
      <hr />
      <div className="recurring-schedule__count">
        <input id={`${id}-has-count`} type="checkbox" checked={value.hasCount} onChange={(e) => onChange({ hasCount: e.target.checked })} />
        <label htmlFor={`${id}-has-count`}>{t('recurring.addAnother')}</label>
        <input
          className="recurring-schedule__tiny"
          type="text"
          inputMode="numeric"
          maxLength={6}
          aria-label={t('recurring.countLabel')}
          aria-invalid={countErrors.length > 0 || undefined}
          value={value.count}
          onChange={(e) => onChange({ count: e.target.value, hasCount: e.target.value.trim() !== '' || value.hasCount })}
        />
        <span>{t('recurring.times')}</span>
      </div>
      {countErrors.length > 0 && <p className="form-input__message form-input__message--error recurring-schedule__message">{countErrors.join(' ')}</p>}
      <div className="form-input form-input--check">
        <span className="form-input__label" aria-hidden="true" />
        <div className="form-input__body recurring-schedule__checks">
          <label className="check">
            <input type="checkbox" checked={value.autoSend} onChange={(e) => onChange({ autoSend: e.target.checked })} />
            <span>{t('recurring.autoSendText')}</span>
          </label>
          {/* Merging only concerns invoices that are sent, so it shows once they are. */}
          {value.autoSend && (
            <label className="check">
              <input type="checkbox" checked={value.mergeable} onChange={(e) => onChange({ mergeable: e.target.checked })} />
              <span>{t('recurring.mergeable')}</span>
            </label>
          )}
        </div>
      </div>
      <hr />
      <div className="recurring-schedule__actions">
        <button className="btn btn--primary" disabled={pending}>{submitLabel}</button>
        {onCancel && <button type="button" className="btn btn--secondary" onClick={onCancel}>{t('common.cancel')}</button>}
      </div>
    </form>
  )
}

type SummaryProps = {
  schedule: Pick<RecurringInvoice, 'invoice_date' | 'frequency' | 'frequency_type' | 'last_date'>
  active: boolean
  /** How many invoices it made, so the next one is numbered after them. */
  made: number
  /** The invoices it made, the latest of which lead the timeline. */
  invoices?: SalesInvoice[]
  administrationId: string
  onEdit?: () => void
}

/** How many of the next invoices the timeline shows by date; dots stand for the ones after. */
const shown = 3

/**
 * Moneybird's sentence over a recurring invoice, "This recurring invoice
 * will be created every month indefinitely. Edit", and the timeline of the
 * invoices it makes.
 */
export function ScheduleSummary({ schedule, active, made, invoices = [], administrationId, onEdit }: SummaryProps) {
  const { t } = useTranslation()
  const frequency = frequencyText(schedule, t)
  const lower = frequency.charAt(0).toLocaleLowerCase() + frequency.slice(1)
  const left = countUntil(schedule)
  const dates = active ? upcoming(schedule, shown) : []
  const more = active && (left === null || left > shown)
  const before = [...invoices].sort((a, b) => (a.invoice_date ?? '').localeCompare(b.invoice_date ?? '')).slice(-2)
  const sentence = !active
    ? t('recurring.sentence.stopped')
    : left === null ? t('recurring.sentence.indefinitely', { frequency: lower }) : t('recurring.sentence.times', { frequency: lower, count: left })
  return (
    <div className="recurring-schedule__summary">
      <h2 className="recurring-schedule__sentence">
        {sentence}
        {onEdit && (
          <>
            {' '}
            <button type="button" className="btn btn--inline recurring-schedule__edit" onClick={onEdit}>{t('recurring.edit')}</button>
          </>
        )}
      </h2>
      {(before.length > 0 || dates.length > 0) && (
        <>
          <hr />
          <ol className="recurring-timeline" aria-label={t('recurring.timeline')}>
            {before.map((inv, i) => (
              <li key={inv.id} className="recurring-timeline__item is-made">
                <span className="recurring-timeline__date">{inv.invoice_date}</span>
                <Link className="recurring-timeline__step" to={`/${administrationId}/sales_invoices/${inv.id}`} aria-label={invoiceTitle(inv, t)} title={invoiceTitle(inv, t)}>
                  {made - before.length + i + 1}
                </Link>
              </li>
            ))}
            {dates.map((d, i) => (
              <li key={d} className={`recurring-timeline__item${i === 0 ? ' is-next' : ''}`}>
                <span className="recurring-timeline__date">{d}</span>
                <span className="recurring-timeline__step">{made + i + 1}</span>
              </li>
            ))}
            {more && <li className="recurring-timeline__more" aria-label={t('recurring.andMore')}><span /><span /><span /><span /><span /><span /><span /><span /></li>}
          </ol>
        </>
      )}
    </div>
  )
}
