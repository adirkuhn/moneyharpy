import { ActionLink } from '../layout/ActionLink'
import { type FormEvent, type ReactNode, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { todayDay } from '../forms/calendar'
import { DateInput } from '../forms/DatePicker'
import { FormRow, formErrors, TextArea } from '../forms/Form'
import { CaretIcon } from '../layout/icons'
import { DangerDialog } from '../layout/Modal'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { ContactPicker } from '../invoices/ContactPicker'
import { valueOn } from './book'
import { type Asset, useAsset, useAssetChange, useDeleteAsset, useDivestAsset } from './queries'
import './assets.css'

/** Loads the asset an action is for. */
function WithAsset({ children }: { children: (asset: Asset) => ReactNode }) {
  const { administrationId = '', assetId = '' } = useParams()
  const asset = useAsset(administrationId, assetId)
  if (asset.error instanceof ApiError && asset.error.status < 500) {
    return <NotFoundPage />
  }
  if (!asset.data) {
    return <Loading />
  }
  return <>{children(asset.data)}</>
}

/** Moneybird's list of what can be done to an asset (/assets/:assetId/value_changes/new). */
export function AssetActionsPage() {
  return <WithAsset>{(asset) => <Actions asset={asset} />}</WithAsset>
}

function Actions({ asset }: { asset: Asset }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { administrationId = '' } = useParams()
  const remove = useDeleteAsset(administrationId, String(asset.id))
  const [confirming, setConfirming] = useState(false)
  const base = `/${administrationId}/assets/${asset.id}`
  const disposed = !!asset.disposal
  const actions: { key: string; to?: string }[] = [
    ...(disposed ? [] : [
      { key: 'arbitrary', to: `${base}/value_changes/arbitrary/new` },
      { key: 'outOfService', to: `${base}/value_changes/full_depreciation/new` },
      { key: 'divestment', to: `${base}/value_changes/divestment/new` },
    ]),
    { key: 'delete' },
  ]
  return (
    <Page wide title={t('assets.actionsTitle')}>
      <Problems messages={formErrors(remove.error, {}).general} />
      <ul className="asset-actions" aria-label={t('assets.actionsTitle')}>
        {actions.map((a) => {
          const body = (
            <>
              <span className="asset-actions__text">
                <span className="asset-actions__title">{t(`assets.do.${a.key}.title`)}</span>
                <span className="asset-actions__help">{t(`assets.do.${a.key}.text`)}</span>
              </span>
              <CaretIcon />
            </>
          )
          return (
            <li key={a.key}>
              {a.to
                ? <Link className="asset-actions__item" to={a.to}>{body}</Link>
                : <button type="button" className="asset-actions__item" onClick={() => setConfirming(true)}>{body}</button>}
            </li>
          )
        })}
      </ul>
      <div className="asset-actions__foot">
        <ActionLink className="btn btn--secondary" to={base}>{t('common.cancel')}</ActionLink>
      </div>
      {confirming && (
        <DangerDialog
          question={t('assets.confirmDeleteTitle')}
          busy={remove.isPending}
          onCancel={() => setConfirming(false)}
          onConfirm={() => remove.mutate(undefined, { onSuccess: () => navigate(`/${administrationId}/assets`) })}
        />
      )}
    </Page>
  )
}

type Kind = 'arbitrary' | 'full_depreciation' | 'divestment'

/** Moneybird's form of one action: what it does, its fields, and Cancel, Back and Save. */
function ActionForm({ asset, kind, onSubmit, pending, error, children }: {
  asset: Asset; kind: Kind; onSubmit: () => void; pending: boolean; error: unknown; children: ReactNode
}) {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const base = `/${administrationId}/assets/${asset.id}`
  const submit = (e: FormEvent) => {
    e.preventDefault()
    onSubmit()
  }
  return (
    <Page wide title={t(`assets.forms.${kind}.title`)}>
      <form className="asset-action-form" onSubmit={submit} noValidate aria-label={t(`assets.forms.${kind}.title`)}>
        <div className="form-sheet asset-form">
          <p className="asset-action-form__intro">{t(`assets.forms.${kind}.intro`)}</p>
          <Problems messages={formErrors(error, {}).general} />
          <div className="asset-action-form__fields">{children}</div>
        </div>
        <div className="asset-action-form__actions">
          <ActionLink className="btn btn--secondary" to={base}>{t('common.cancel')}</ActionLink>
          <ActionLink className="btn btn--secondary" to={`${base}/value_changes/new`}>{t('assets.back')}</ActionLink>
          <button className="btn btn--primary" disabled={pending}>{t('assets.save')}</button>
        </div>
      </form>
    </Page>
  )
}

function MoneyRow({ label, value, onChange, errors, help }: { label: string; value: string; onChange: (v: string) => void; errors?: string[]; help?: string }) {
  return (
    <FormRow label={label} width="small" errors={errors} help={help}>
      {({ labelId: _, ...control }) => (
        <span className="asset-money">
          <span className="asset-money__currency" aria-hidden="true">€</span>
          <input {...control} type="text" inputMode="decimal" placeholder="0.00" value={value} onChange={(e) => onChange(e.target.value)} />
        </span>
      )}
    </FormRow>
  )
}

/** Arbitrary depreciation (/assets/:assetId/value_changes/arbitrary/new). */
export function ArbitraryDepreciationPage() {
  return <WithAsset>{(asset) => <Arbitrary asset={asset} />}</WithAsset>
}

function Arbitrary({ asset }: { asset: Asset }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { administrationId = '' } = useParams()
  const save = useAssetChange(administrationId, String(asset.id), 'arbitrary')
  const [description, setDescription] = useState('')
  const [amount, setAmount] = useState('')
  const [date, setDate] = useState(todayDay())
  const labels = { description: t('assets.description'), amount: t('assets.depreciation'), date: t('assets.date') }
  const errors = formErrors(save.error, labels)
  return (
    <ActionForm
      asset={asset} kind="arbitrary" pending={save.isPending} error={save.error}
      onSubmit={() => save.mutate(
        { date, description: description.trim(), amount: amount.trim() ? `-${amount.trim().replace(/^-/, '')}` : '' },
        { onSuccess: () => navigate(`/${administrationId}/assets/${asset.id}`) },
      )}
    >
      <TextArea label={labels.description} placeholder={t('assets.forms.arbitrary.placeholder')} value={description} errors={errors.field('description')} onChange={(e) => setDescription(e.target.value)} />
      <MoneyRow label={labels.amount} value={amount} onChange={setAmount} errors={errors.field('amount')} />
      <DateInput label={labels.date} value={date} errors={errors.field('date')} onChange={setDate} />
    </ActionForm>
  )
}

/** Take out of service, Moneybird's full depreciation (/assets/:assetId/value_changes/full_depreciation/new). */
export function OutOfServicePage() {
  return <WithAsset>{(asset) => <OutOfService asset={asset} />}</WithAsset>
}

function OutOfService({ asset }: { asset: Asset }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { administrationId = '' } = useParams()
  const save = useAssetChange(administrationId, String(asset.id), 'full_depreciation')
  const [description, setDescription] = useState('')
  const [date, setDate] = useState(todayDay())
  const labels = { description: t('assets.description'), date: t('assets.date') }
  const errors = formErrors(save.error, labels)
  return (
    <ActionForm
      asset={asset} kind="full_depreciation" pending={save.isPending} error={save.error}
      onSubmit={() => save.mutate({ date, description: description.trim() }, { onSuccess: () => navigate(`/${administrationId}/assets/${asset.id}`) })}
    >
      <TextArea label={labels.description} value={description} errors={errors.field('description')} onChange={(e) => setDescription(e.target.value)} />
      <DateInput label={labels.date} value={date} errors={errors.field('date')} onChange={setDate} />
    </ActionForm>
  )
}

/** Divestment, with a draft sales invoice for the sale (/assets/:assetId/value_changes/divestment/new). */
export function DivestmentPage() {
  return <WithAsset>{(asset) => <Divestment asset={asset} />}</WithAsset>
}

function Divestment({ asset }: { asset: Asset }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { administrationId = '' } = useParams()
  const divest = useDivestAsset(administrationId, asset)
  const [date, setDate] = useState(todayDay())
  const [invoice, setInvoice] = useState(true)
  const [contact, setContact] = useState<{ id: string; name: string } | null>(null)
  const [typed, setTyped] = useState<string | null>(null)
  // Moneybird's amount starts at the value on the day, as the divestment books it.
  const amount = typed ?? valueOn(asset, date).toFixed(2)
  const labels = { date: t('assets.date'), contact_id: t('assets.contact'), amount: t('assets.amount') }
  const errors = formErrors(divest.error, labels)
  return (
    <ActionForm
      asset={asset} kind="divestment" pending={divest.isPending} error={divest.error}
      onSubmit={() => divest.mutate(
        { date, invoice: invoice ? { contactId: contact?.id ?? '', amount: amount.trim() } : undefined },
        { onSuccess: (made) => navigate(made ? `/${administrationId}/sales_invoices/${made}` : `/${administrationId}/assets/${asset.id}`) },
      )}
    >
      <DateInput label={labels.date} value={date} errors={errors.field('date')} onChange={setDate} />
      <hr className="asset-action-form__rule" />
      <FormRow label={t('assets.forms.divestment.createInvoice')} help={t('assets.forms.divestment.createInvoiceHelp')}>
        {(control) => (
          <input {...control} type="checkbox" role="switch" className="switch" checked={invoice} onChange={(e) => setInvoice(e.target.checked)} />
        )}
      </FormRow>
      {invoice && (
        <>
          <ContactPicker
            administrationId={administrationId} label={labels.contact_id} value={contact?.name ?? ''} errors={errors.field('contact_id')}
            onChange={(c) => setContact({ id: String(c.id ?? ''), name: String(c.company_name || [c.firstname, c.lastname].filter(Boolean).join(' ')) })}
          />
          <MoneyRow label={labels.amount} value={amount} onChange={setTyped} errors={errors.field('amount')} />
        </>
      )}
    </ActionForm>
  )
}
