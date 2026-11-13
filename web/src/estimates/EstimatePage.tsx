import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { formatAmount } from '../banking/queries'
import { type TaxRate, useLedgerAccounts, useTaxRates } from '../bookkeeping/queries'
import { contactName } from '../contacts/queries'
import { ConfirmPanel, DetailBlock, DetailList, type DetailRow } from '../detail/Detail'
import { DateInput } from '../forms/DatePicker'
import { CheckboxInput, FormSection, formErrors, SelectInput, TextArea, TextInput } from '../forms/Form'
import { ContactPicker } from '../invoices/ContactPicker'
import { type LineRow, LinesEditor } from '../invoices/LinesEditor'
import { computeTotals, parseCents, parsePercentage, parseQuantity } from '../invoices/totals'
import { TotalsTable } from '../invoices/TotalsTable'
import { useDocumentStyles, useWorkflows } from '../invoicing/queries'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { type MenuItem, MoreMenu } from '../list/ListActions'
import { EstimateBadge } from './EstimatesPage'
import {
  type Estimate, type EstimateChanges, estimatePdfURL, estimateTitle, isBillable, isEditable, transitions, useBillEstimate, useChangeEstimateState,
  useDeleteEstimate, useEstimate, useSaveEstimate, useSendEstimate,
} from './queries'

/** One estimate (/estimates/:id), or a new one (/estimates/new). */
export function EstimatePage() {
  const { administrationId = '', estimateId } = useParams()
  const estimate = useEstimate(administrationId, estimateId)
  const rates = useTaxRates(administrationId)
  if (estimateId && estimate.error instanceof ApiError && estimate.error.status < 500) {
    return <NotFoundPage />
  }
  if ((estimateId && !estimate.data) || !rates.data) {
    return <Loading />
  }
  const e = estimate.data ?? null
  const usable = rates.data.filter((r) => r.tax_rate_type === 'sales_invoice' && (r.active || e?.details?.some((d) => d.tax_rate_id === r.id)))
  if (!e) {
    return <NewEstimate administrationId={administrationId} rates={usable} />
  }
  return <EstimateView key={e.id} administrationId={administrationId} estimate={e} rates={usable} />
}

function NewEstimate({ administrationId, rates }: { administrationId: string; rates: TaxRate[] }) {
  const { t } = useTranslation()
  return (
    <Page title={t('estimates.newTitle')} back={{ to: `/${administrationId}/estimates`, label: t('estimates.backToList') }}>
      <EstimateForm administrationId={administrationId} estimate={null} rates={rates} />
    </Page>
  )
}

function EstimateView({ administrationId, estimate: e, rates }: { administrationId: string; estimate: Estimate; rates: TaxRate[] }) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const id = e.id ?? ''
  const send = useSendEstimate(administrationId, id)
  const change = useChangeEstimateState(administrationId, id)
  const bill = useBillEstimate(administrationId, id)
  const remove = useDeleteEstimate(administrationId, id)
  const [confirming, setConfirming] = useState(false)
  const list = `/${administrationId}/estimates`
  const state = e.state ?? 'draft'
  const date = (d: string | null | undefined) => d && new Date(`${d}T00:00:00`).toLocaleDateString(i18n.language, { dateStyle: 'medium' })

  const more: MenuItem[] = [
    ...(state !== 'draft' ? [{ label: t('estimates.sendAgain'), disabled: send.isPending, onSelect: () => send.mutate({}) }] : []),
    ...transitions[state].map((to) => ({ label: t(`estimates.mark.${to}`), disabled: change.isPending, onSelect: () => change.mutate(to) })),
    { label: t('estimates.downloadPdf'), href: estimatePdfURL(administrationId, id), download: true },
    { label: t('estimates.delete'), danger: true, onSelect: () => setConfirming(true) },
  ]
  const rows: DetailRow[] = [
    [t('estimates.status'), <EstimateBadge key="state" state={state} />],
    [t('estimates.number'), estimateTitle(e, t)],
    [t('estimates.date'), date(e.estimate_date)],
    [t('estimates.validUntil'), state !== 'draft' && date(e.due_date)],
    [t('invoices.amount'), formatAmount(e.total_price_incl_tax, e.currency ?? 'EUR', i18n.language)],
  ]

  return (
    <Page
      title={estimateTitle(e, t)}
      subtitle={e.contact ? contactName(e.contact) : undefined}
      back={{ to: list, label: t('estimates.backToList') }}
      actions={
        <>
          {state === 'draft' && (
            <>
              <button type="button" className="btn btn--secondary" disabled={send.isPending} onClick={() => send.mutate({ delivery_method: 'Manual' })}>
                {t('estimates.markSent')}
              </button>
              <button type="button" className="btn btn--primary" disabled={send.isPending} onClick={() => send.mutate({ delivery_method: 'Email' })}>
                {t('estimates.send')}
              </button>
            </>
          )}
          {isBillable(state) && (
            <button
              type="button"
              className="btn btn--primary"
              disabled={bill.isPending}
              onClick={() => bill.mutate(undefined, { onSuccess: (inv) => navigate(`/${administrationId}/sales_invoices/${inv.id}`) })}
            >
              {t('estimates.bill')}
            </button>
          )}
          <MoreMenu items={more} />
        </>
      }
    >
      <Problems messages={[...formErrors(send.error ?? change.error ?? bill.error ?? remove.error, {}).general]} />
      {confirming && (
        <ConfirmPanel
          message={t('estimates.confirmDeleteText')}
          confirmLabel={t('estimates.confirmDelete')}
          disabled={remove.isPending}
          onConfirm={() => remove.mutate(undefined, { onSuccess: () => navigate(list) })}
          onCancel={() => setConfirming(false)}
        />
      )}
      <DetailBlock title={t('estimates.detailsTitle')}>
        <DetailList rows={rows} />
      </DetailBlock>
      <DetailBlock title={t('estimates.editTitle')}>
        {!isEditable(state) && <p className="notice">{t('estimates.lockedNotice')}</p>}
        {/* A fresh form for every version, so saved lines carry their ids. */}
        <EstimateForm key={e.updated_at} administrationId={administrationId} estimate={e} rates={rates} />
      </DetailBlock>
    </Page>
  )
}

let nextKey = 0
const newKey = () => `new-${++nextKey}`
const blank = (taxRateId: string): LineRow => ({ key: newKey(), description: '', amount: '1', price: '', tax_rate_id: taxRateId, ledger_account_id: '' })
/** Shows "100.0" from the server as "100" and keeps "12.5" as it is. */
const priceText = (p: string | undefined) => (p === undefined || p === '' ? '' : String(Number(p)))

type FormProps = { administrationId: string; estimate: Estimate | null; rates: TaxRate[] }

/** The contact, texts and lines of an estimate, with the invoice line editor; billed and archived ones show them without changing. */
function EstimateForm({ administrationId, estimate, rates }: FormProps) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const save = useSaveEstimate(administrationId, estimate?.id)
  const administration = useAdministration(administrationId)
  const accounts = useLedgerAccounts(administrationId)
  const workflows = useWorkflows(administrationId)
  const styles = useDocumentStyles(administrationId)
  const defaultRate = rates.find((r) => r.active)?.id ?? ''
  const locked = estimate !== null && !isEditable(estimate.state)

  const [contact, setContact] = useState({ id: estimate?.contact_id ?? '', name: estimate?.contact ? contactName(estimate.contact) : '' })
  const [reference, setReference] = useState(estimate?.reference ?? '')
  const [date, setDate] = useState(estimate?.estimate_date ?? '')
  const [validity, setValidity] = useState(estimate?.first_due_interval === undefined ? '' : String(estimate.first_due_interval))
  const [workflowId, setWorkflowId] = useState(estimate?.workflow_id ?? '')
  const [styleId, setStyleId] = useState(estimate?.document_style_id ?? '')
  const [inclTax, setInclTax] = useState<boolean | null>(estimate?.prices_are_incl_tax ?? null)
  const [discount, setDiscount] = useState(estimate?.discount && Number(estimate.discount) !== 0 ? String(Number(estimate.discount)) : '')
  const [preText, setPreText] = useState(estimate?.pre_text ?? '')
  const [postText, setPostText] = useState(estimate?.post_text ?? '')
  const [rows, setRows] = useState<LineRow[]>(() =>
    estimate?.details?.length
      ? estimate.details.map((d) => ({
          key: d.id ?? newKey(), id: d.id, description: d.description ?? '', amount: d.amount ?? '1', price: priceText(d.price),
          tax_rate_id: d.tax_rate_id ?? defaultRate, ledger_account_id: d.ledger_account_id ?? '',
        }))
      : [blank(defaultRate)],
  )
  const [removed, setRemoved] = useState<string[]>([])

  const flows = (workflows.data ?? []).filter((w) => w.type === 'EstimateWorkflow' && (w.active || w.id === workflowId))
  const flow = flows.find((w) => w.id === workflowId) ?? flows.find((w) => w.default)
  const pricesInclTax = inclTax ?? flow?.prices_are_incl_tax ?? false
  const currency = estimate?.currency ?? flow?.currency ?? administration.data?.currency ?? 'EUR'
  const money = (cents: number) => formatAmount(cents / 100, currency, i18n.language)
  const parsed = rows.map((r) => ({ price: parseCents(r.price), quantity: parseQuantity(r.amount) }))
  const percentage = (id: string) => Math.round(Number(rates.find((r) => r.id === id)?.percentage ?? 0) * 100)
  const discountValue = parsePercentage(discount)
  const totals = computeTotals(
    rows.map((r, i) => ({ price: parsed[i]?.price ?? 0, quantity: parsed[i]?.quantity ?? 0, taxRateId: r.tax_rate_id, rate: percentage(r.tax_rate_id) })),
    discountValue ?? 0,
    pricesInclTax,
  )
  const rateName = (id: string) => rates.find((r) => r.id === id)?.name ?? ''

  const labels: Record<string, string> = {
    contact_id: t('invoices.contact'), reference: t('invoices.reference'), estimate_date: t('estimates.date'), first_due_interval: t('estimates.validity'),
    workflow_id: t('invoices.workflow'), document_style_id: t('invoices.documentStyle'), discount: t('invoices.discount'),
    prices_are_incl_tax: t('invoices.pricesInclTax'), pre_text: t('estimates.preText'), post_text: t('estimates.postText'), details_attributes: t('invoices.lines'),
  }
  rows.forEach((_, i) => {
    for (const [field, name] of Object.entries({ description: t('invoices.description'), amount: t('invoices.quantity'), price: t('invoices.price'), tax_rate_id: t('invoices.vatRate'), ledger_account_id: t('invoices.category') })) {
      labels[`details_attributes.${i}.${field}`] = t('invoices.lineField', { field: name, n: i + 1 })
    }
  })
  const errors = formErrors(save.error, labels)

  const change = (i: number, c: Partial<LineRow>) => setRows(rows.map((r, j) => (j === i ? { ...r, ...c } : r)))
  const remove = (i: number) => {
    const row = rows[i]
    if (row?.id) {
      setRemoved([...removed, row.id])
    }
    setRows(rows.filter((_, j) => j !== i))
  }

  const submit = (e: FormEvent) => {
    e.preventDefault()
    // A new line left empty is not meant as a line.
    const kept = rows.filter((r) => r.id || r.description.trim() || r.price.trim())
    const body: EstimateChanges = {
      contact_id: contact.id,
      reference: reference.trim(),
      estimate_date: date,
      prices_are_incl_tax: pricesInclTax,
      discount: discount.trim(),
      pre_text: preText,
      post_text: postText,
      ...(validity.trim() !== '' ? { first_due_interval: Number(validity) } : {}),
      ...(workflowId ? { workflow_id: workflowId } : {}),
      ...(styleId ? { document_style_id: styleId } : {}),
      details_attributes: [
        ...kept.map((r) => ({
          ...(r.id ? { id: r.id } : {}),
          description: r.description.trim(),
          amount: r.amount.trim(),
          price: r.price.trim(),
          tax_rate_id: r.tax_rate_id,
          ...(r.ledger_account_id ? { ledger_account_id: r.ledger_account_id } : {}),
        })),
        ...removed.map((id) => ({ id, _destroy: true })),
      ],
    }
    save.mutate(body, {
      onSuccess: (saved) => {
        setRemoved([])
        if (!estimate) {
          navigate(`/${administrationId}/estimates/${saved.id}`)
        }
      },
    })
  }

  const totalRows = [
    { label: t('invoices.subtotal'), amount: money(totals.subtotal) },
    ...(totals.discount !== 0 ? [{ label: t('invoices.discountAmount', { percentage: discount.trim() }), amount: money(-totals.discount) }] : []),
    ...totals.byRate.map((r) => ({ label: pricesInclTax ? t('invoices.includesVat', { name: rateName(r.taxRateId) }) : rateName(r.taxRateId), amount: money(r.tax) })),
    { label: t('invoices.total'), amount: money(totals.inclTax), strong: true },
  ]

  return (
    <form className="form-sheet invoice-form" onSubmit={submit} noValidate aria-label={t('estimates.form')}>
      <fieldset className="form-sheet__fieldset" disabled={locked}>
        <Problems messages={[...errors.general, ...(errors.field('details_attributes') ?? [])]} />
        <FormSection>
          <ContactPicker
            administrationId={administrationId}
            label={labels.contact_id ?? ''}
            value={contact.name}
            errors={errors.field('contact_id')}
            onChange={(c) => {
              setContact({ id: String(c.id ?? ''), name: contactName(c) })
              if (!estimate && !workflowId && c.estimate_workflow_id) {
                setWorkflowId(String(c.estimate_workflow_id))
              }
            }}
          />
          <TextInput label={labels.reference ?? ''} maxLength={1000} value={reference} errors={errors.field('reference')} onChange={(e) => setReference(e.target.value)} />
          <DateInput label={labels.estimate_date ?? ''} width="medium" help={t('estimates.dateHelp')} value={date} errors={errors.field('estimate_date')} onChange={setDate} />
        </FormSection>
        <FormSection title={t('estimates.settings')}>
          <SelectInput label={labels.workflow_id ?? ''} width="medium" value={workflowId} errors={errors.field('workflow_id')} onChange={(e) => setWorkflowId(e.target.value)}>
            {!estimate && <option value="">{t('invoices.defaultWorkflow')}</option>}
            {flows.map((w) => <option key={w.id} value={w.id}>{w.name}</option>)}
          </SelectInput>
          <SelectInput label={labels.document_style_id ?? ''} width="medium" value={styleId} errors={errors.field('document_style_id')} onChange={(e) => setStyleId(e.target.value)}>
            {!estimate && <option value="">{t('invoices.defaultStyle')}</option>}
            {(styles.data ?? []).map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
          </SelectInput>
          <TextInput
            label={labels.first_due_interval ?? ''}
            type="number"
            inputMode="numeric"
            min={0}
            max={365}
            width="small"
            placeholder={String(flow?.steps?.[0]?.due_interval ?? '')}
            help={t('estimates.validityHelp')}
            value={validity}
            errors={errors.field('first_due_interval')}
            onChange={(e) => setValidity(e.target.value)}
          />
          <CheckboxInput label={labels.prices_are_incl_tax ?? ''} checked={pricesInclTax} onChange={(e) => setInclTax(e.target.checked)} />
          <TextInput
            label={labels.discount ?? ''}
            width="small"
            inputMode="decimal"
            maxLength={10}
            help={t('invoices.discountHelp')}
            value={discount}
            aria-invalid={discountValue === null || undefined}
            errors={errors.field('discount')}
            onChange={(e) => setDiscount(e.target.value)}
          />
        </FormSection>
        <FormSection title={t('invoices.lines')}>
          <TextArea label={labels.pre_text ?? ''} maxLength={10000} value={preText} errors={errors.field('pre_text')} onChange={(e) => setPreText(e.target.value)} />
          <LinesEditor
            rows={rows}
            rates={rates}
            accounts={accounts.data ?? []}
            amounts={totals.lines.map(money)}
            invalid={parsed.map((p) => ({ price: p.price === null, amount: p.quantity === null }))}
            errors={errors}
            onChange={change}
            onRemove={remove}
            onAdd={() => setRows([...rows, blank(defaultRate)])}
          />
          <TotalsTable rows={totalRows} />
          <TextArea label={labels.post_text ?? ''} maxLength={10000} value={postText} errors={errors.field('post_text')} onChange={(e) => setPostText(e.target.value)} />
        </FormSection>
        {!locked && (
          <div className="form-sheet__actions">
            <button className="btn btn--primary" disabled={save.isPending}>{t('estimates.save')}</button>
            {!estimate && <Link className="btn btn--secondary" to={`/${administrationId}/estimates`}>{t('common.cancel')}</Link>}
          </div>
        )}
      </fieldset>
    </form>
  )
}
