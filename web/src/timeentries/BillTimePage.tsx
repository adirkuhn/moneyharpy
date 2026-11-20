import { ActionLink } from '../layout/ActionLink'
import { type FormEvent, Fragment, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { useLedgerAccounts, useTaxRates } from '../bookkeeping/queries'
import { formatAmount } from '../banking/queries'
import { SegmentedRow } from '../contacts/ContactFormParts'
import { contactName as nameOf } from '../contacts/queries'
import { FormRow, formErrors, SelectInput } from '../forms/Form'
import { RadioRow } from '../forms/RadioRow'
import { ContactPicker } from '../invoices/ContactPicker'
import { useSalesInvoices, useSaveSalesInvoice } from '../invoices/queries'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { AdvancedFilter, CheckFilter, FilterPanel } from '../list/FilterPanel'
import { parsePeriod, periodValue } from '../list/period'
import { PeriodPicker } from '../list/PeriodPicker'
import { billDetails, type DetailPart, detailParts, type Grouping, groupings } from './billing'
import { contactName } from './TimeEntriesPage'
import { type TimeEntry, useReportEntries, workedMinutes } from './queries'
import { clock, dayOf, weekOf } from './week'
import './timesheets.css'

type Contact = { id: string; name: string }

/**
 * Moneybird's "Time entries on sales invoice" (/time_entries_wizard): in
 * three steps, the contact to bill, the open time to bill them for, and the
 * invoice it goes on, new or existing, with how its lines are made.
 */
export function BillTimePage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const administration = useAdministration(administrationId)
  const [step, setStep] = useState<1 | 2 | 3>(1)
  const [contact, setContact] = useState<Contact | null>(null)
  const [chosen, setChosen] = useState<Set<string>>(new Set())
  const [period, setPeriod] = useState('this_year')
  const [withoutContact, setWithoutContact] = useState(true)
  const filter = `state:open,period:${period},include_active:false,contact_id:${contact?.id ?? ''}${withoutContact ? ',include_nil_contacts:true' : ''}`
  const entries = useReportEntries(administrationId, filter, '', !!contact && step > 1)

  if (administration.error instanceof ApiError && administration.error.status < 500) {
    return <NotFoundPage />
  }
  if (!administration.data) {
    return <Loading />
  }
  const report = `/${administrationId}/time_entries_report`
  const open = entries.data ?? []
  const picked = open.filter((e) => chosen.has(e.id ?? ''))
  const cancel = <ActionLink className="btn btn--secondary" to={report}>{t('common.cancel')}</ActionLink>
  const back = (to: 1 | 2) => <button type="button" className="btn btn--secondary" onClick={() => setStep(to)}>{t('timeEntries.bill.previous')}</button>

  if (step === 1) {
    return (
      <Page wide title={t('timeEntries.bill.title')}>
        <form className="bill-step" aria-label={t('timeEntries.bill.contactStep')} onSubmit={(e) => { e.preventDefault(); if (contact) setStep(2) }}>
          <div className="bill-step__card">
            <h2 className="bill-step__question">{t('timeEntries.bill.whichContact')}</h2>
            <div className="bill-step__body">
            <ContactPicker
              administrationId={administrationId}
              label={t('timeEntries.contact')}
              value={contact?.name ?? ''}
              onChange={(c) => {
                setContact({ id: c.id ?? '', name: nameOf(c) })
                setChosen(new Set())
              }}
            />
            </div>
          </div>
          <BottomBar step={1}>
            {cancel}
            <button className="btn btn--primary" disabled={!contact}>{t('timeEntries.bill.next')}</button>
          </BottomBar>
        </form>
      </Page>
    )
  }

  if (step === 2 || !contact) {
    return (
      <Page
        wide
        hideTitle
        title={t('timeEntries.bill.title')}
        lead={
          <FilterPanel
            label={period === 'this_year' ? t('timeEntries.bill.openOf', { name: contact?.name ?? '' }) : t('timeEntries.bill.openOfFiltered', { name: contact?.name ?? '' })}
            title={t('timeEntries.report.filters')}
            quick={[]}
          >
            <TimeFilter period={period} withoutContact={withoutContact} onApply={(p, w) => { setPeriod(p); setWithoutContact(w) }} />
          </FilterPanel>
        }
      >
        <form aria-label={t('timeEntries.bill.timeStep')} onSubmit={(e) => { e.preventDefault(); if (picked.length > 0) setStep(3) }}>
          {!entries.data ? <Loading /> : <PickTable entries={open} chosen={chosen} onChange={setChosen} />}
          <BottomBar step={2} status={t('timeEntries.bill.selected', { count: picked.length })}>
            {cancel}
            {back(1)}
            <button className="btn btn--primary" disabled={picked.length === 0}>{t('timeEntries.bill.next')}</button>
          </BottomBar>
        </form>
      </Page>
    )
  }

  return (
    <Page wide title={t('timeEntries.bill.title')}>
      <InvoiceStep administrationId={administrationId} contact={contact} entries={picked} cancel={cancel} back={back(2)} />
    </Page>
  )
}

/** The step's place in the wizard, its status and its buttons, after Moneybird's bar at the bottom. */
function BottomBar({ step, status, children }: { step: number; status?: string; children: React.ReactNode }) {
  const { t } = useTranslation()
  return (
    <div className="bill-bar">
      <div className="bill-bar__stepper">
        <span>{t('timeEntries.bill.step', { step, of: 3 })}</span>
        <ol className="bill-bar__steps" aria-hidden="true">
          {[1, 2, 3].map((n) => <li key={n} className={n < step ? 'is-done' : n === step ? 'is-active' : ''} />)}
        </ol>
      </div>
      {status && <p className="bill-bar__status" role="status">{status}</p>}
      <div className="bill-bar__actions">{children}</div>
    </div>
  )
}

/** The period of the open time, and whether time without a contact shows too. */
function TimeFilter({ period, withoutContact, onApply }: { period: string; withoutContact: boolean; onApply: (period: string, withoutContact: boolean) => void }) {
  const { t } = useTranslation()
  const today = useMemo(() => new Date(), [])
  const [range, setRange] = useState(() => parsePeriod(period, today))
  const [without, setWithout] = useState(withoutContact)
  return (
    <AdvancedFilter value={{ range, without }} onApply={() => onApply(periodValue(range, today) || 'this_year', without)}>
      <PeriodPicker value={range} today={today} onChange={setRange} />
      <CheckFilter label={t('timeEntries.bill.withoutContact')} checked={without} onChange={setWithout} />
    </AdvancedFilter>
  )
}

/** The open time week by week, each with a checkbox, and one to choose all. */
function PickTable({ entries, chosen, onChange }: { entries: TimeEntry[]; chosen: Set<string>; onChange: (next: Set<string>) => void }) {
  const { t } = useTranslation()
  const time = (iso: string | null | undefined) => (iso ? clock(new Date(iso).getHours() * 60 + new Date(iso).getMinutes()) : '')
  const all = entries.length > 0 && entries.every((e) => chosen.has(e.id ?? ''))
  const toggle = (id: string) => {
    const next = new Set(chosen)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    onChange(next)
  }
  const weeks: { key: string; week: number; list: TimeEntry[] }[] = []
  for (const e of entries) {
    const { year, week } = weekOf(dayOf(e.started_at))
    const last = weeks.at(-1)
    if (last?.key === `${year}-${week}`) last.list.push(e)
    else weeks.push({ key: `${year}-${week}`, week, list: [e] })
  }
  return (
    <table className="data-table timesheet-table hours-report" aria-label={t('timeEntries.bill.timeStep')}>
      <thead>
        <tr>
          <th scope="col" className="timesheet-table__check">
            <input type="checkbox" aria-label={t('timeEntries.bill.selectAll')} checked={all} disabled={entries.length === 0} onChange={() => onChange(all ? new Set() : new Set(entries.map((e) => e.id ?? '')))} />
          </th>
          <th scope="col">{t('timeEntries.date')}</th>
          <th scope="col">{t('timeEntries.report.period')}</th>
          <th scope="col">{t('timeEntries.project')}</th>
          <th scope="col">{t('timeEntries.contact')}</th>
          <th scope="col">{t('timeEntries.description')}</th>
          <th scope="col">{t('timeEntries.report.employee')}</th>
          <th scope="col" className="timesheet-table__amount">{t('timeEntries.report.total')}</th>
        </tr>
      </thead>
      <tbody>
        {entries.length === 0 && (
          <tr><td colSpan={8} className="timesheet-table__empty">{t('timeEntries.bill.nothingOpen')}</td></tr>
        )}
        {weeks.map((w) => (
          <Fragment key={w.key}>
            <tr className="hours-report__week">
              <td />
              <th scope="rowgroup" colSpan={6}>{t('timeEntries.report.week', { week: w.week })}</th>
              <td className="timesheet-table__amount">{clock(w.list.reduce((sum, e) => sum + workedMinutes(e), 0))}</td>
            </tr>
            {w.list.map((e) => (
              <tr key={e.id}>
                <td className="timesheet-table__check">
                  <input
                    type="checkbox"
                    aria-label={t('timeEntries.bill.choose', { date: dayOf(e.started_at), description: e.description })}
                    checked={chosen.has(e.id ?? '')}
                    onChange={() => toggle(e.id ?? '')}
                  />
                </td>
                <td>{dayOf(e.started_at)}</td>
                <td>{`${time(e.started_at)} - ${time(e.ended_at)}`}</td>
                <td>{e.project?.name ?? ''}</td>
                <td>{contactName(e)}</td>
                <td>{e.description}</td>
                <td>{e.user?.name ?? ''}</td>
                <td className="timesheet-table__amount">{clock(workedMinutes(e))}</td>
              </tr>
            ))}
          </Fragment>
        ))}
      </tbody>
    </table>
  )
}

type InvoiceProps = { administrationId: string; contact: Contact; entries: TimeEntry[]; cancel: React.ReactNode; back: React.ReactNode }

/**
 * The last step: a new invoice or one of the contact's, whether the time
 * adds lines to it, how the lines are made, the hourly rate and the category.
 */
function InvoiceStep({ administrationId, contact, entries, cancel, back }: InvoiceProps) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const rates = useTaxRates(administrationId)
  const accounts = useLedgerAccounts(administrationId)
  const invoices = useSalesInvoices(administrationId, `contact_id:${contact.id},state:all`, 1)
  const [kind, setKind] = useState<'new_invoice' | 'existing_invoice'>('new_invoice')
  const [invoiceId, setInvoiceId] = useState('')
  const [addDetails, setAddDetails] = useState(true)
  const [grouping, setGrouping] = useState<Grouping>('combine_all')
  const [parts, setParts] = useState<DetailPart[]>(['description'])
  const [rate, setRate] = useState('')
  const [ledger, setLedger] = useState('')
  const existing = invoices.data?.invoices ?? []
  const invoice = existing.find((i) => i.id === (invoiceId || existing[0]?.id))
  const save = useSaveSalesInvoice(administrationId, kind === 'existing_invoice' ? invoice?.id : undefined)
  const errors = formErrors(save.error, { price: t('timeEntries.bill.rate') })
  // A paid invoice keeps its lines: the time is only linked to it.
  const canAdd = kind === 'new_invoice' || (invoice !== undefined && !['paid', 'uncollectible'].includes(invoice.state ?? ''))
  const taxRate = (rates.data ?? []).find((r) => r.tax_rate_type === 'sales_invoice' && r.active)?.id ?? ''
  const revenue = (accounts.data ?? []).filter((a) => a.account_type === 'revenue')

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const price = rate.trim().replace(',', '.') || '0'
    const details = billDetails(entries, grouping, parts, { price, taxRateId: taxRate, ledgerAccountId: ledger })
    const ids = entries.map((x) => x.id ?? '')
    const lines = canAdd && (kind === 'new_invoice' || addDetails) ? { details_attributes: details } : {}
    save.mutate(
      kind === 'new_invoice' ? { contact_id: contact.id, time_entry_ids: ids, ...lines } : { time_entry_ids: ids, ...lines },
      { onSuccess: (saved) => navigate(`/${administrationId}/sales_invoices/${saved.id}`) },
    )
  }

  return (
    <form className="bill-step" aria-label={t('timeEntries.bill.invoiceStep')} onSubmit={submit}>
      <div className="bill-step__card">
      <h2 className="bill-step__question">{t('timeEntries.bill.how')}</h2>
      <div className="bill-step__body bill-step__body--narrow">
      <Problems messages={[...errors.general, ...(errors.field('price') ?? [])]} />
      <div className="bill-step__kind">
        <SegmentedRow
          label={t('timeEntries.bill.kind')}
          value={kind}
          options={[{ value: 'new_invoice', label: t('timeEntries.bill.newInvoice') }, { value: 'existing_invoice', label: t('timeEntries.bill.existingInvoice') }]}
          onChange={setKind}
        />
      </div>
        {kind === 'existing_invoice' && (
          <>
            <SelectInput label={t('timeEntries.invoice')} value={invoice?.id ?? ''} onChange={(e) => setInvoiceId(e.target.value)} disabled={existing.length === 0}>
              {existing.length === 0 && <option value="">{t('timeEntries.bill.noInvoice')}</option>}
              {existing.map((i) => (
                <option key={i.id} value={i.id}>
                  {[i.invoice_id ?? t('timeEntries.draftInvoice'), t(`invoices.states.${i.state}`, { defaultValue: i.state ?? '' }), i.invoice_date ?? '', formatAmount(i.total_price_incl_tax, i.currency ?? 'EUR', i18n.language)].filter(Boolean).join(' · ')}
                </option>
              ))}
            </SelectInput>
            <div className="form-input form-input--check">
              <span className="form-input__label">{t('timeEntries.bill.addDetails')}</span>
              <div className="form-input__body">
                <label className="check">
                  <input type="checkbox" checked={canAdd && addDetails} disabled={!canAdd} onChange={(e) => setAddDetails(e.target.checked)} />
                  <span>{t('timeEntries.bill.addDetails')}</span>
                </label>
              </div>
            </div>
            <hr />
          </>
        )}
        <RadioRow
          label={t('timeEntries.bill.markup')}
          value={grouping}
          options={groupings.map((g) => ({ value: g, label: t(`timeEntries.bill.groupings.${g}`) }))}
          onChange={setGrouping}
        />
        {grouping === 'group_by' && (
          <fieldset className="form-input bill-step__parts">
            <legend className="form-input__label">{t('timeEntries.bill.parts')}</legend>
            <div className="form-input__body">
              {detailParts.map((p) => (
                <label key={p} className="check">
                  <input type="checkbox" checked={parts.includes(p)} onChange={() => setParts(parts.includes(p) ? parts.filter((x) => x !== p) : [...parts, p])} />
                  <span>{t(`timeEntries.bill.partNames.${p}`)}</span>
                </label>
              ))}
            </div>
          </fieldset>
        )}
        <FormRow label={t('timeEntries.bill.rate')} width="small">
          {({ labelId: _, ...control }) => (
            <span className="bill-step__money">
              <span className="bill-step__currency" aria-hidden="true">€</span>
              <input {...control} type="text" inputMode="decimal" placeholder="0.00" maxLength={16} value={rate} onChange={(e) => setRate(e.target.value)} />
            </span>
          )}
        </FormRow>
        <SelectInput label={t('timeEntries.bill.category')} width="medium" value={ledger} onChange={(e) => setLedger(e.target.value)}>
          <option value="">{t('invoices.uncategorized')}</option>
          {revenue.map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}
        </SelectInput>
      </div>
      </div>
      <BottomBar step={3}>
        {cancel}
        {back}
        <button className="btn btn--primary" disabled={save.isPending || !taxRate || (kind === 'existing_invoice' && !invoice)}>{t('timeEntries.bill.create')}</button>
      </BottomBar>
    </form>
  )
}
