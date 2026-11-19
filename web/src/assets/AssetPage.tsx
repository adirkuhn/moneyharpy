import { Fragment, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { formatAmount } from '../banking/queries'
import { useLedgerAccounts } from '../bookkeeping/queries'
import { ConfirmPanel } from '../detail/Detail'
import { formErrors } from '../forms/Form'
import { NotesAndHistory } from '../invoices/InvoiceHistory'
import { CaretIcon, ListIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { MoreMenu } from '../list/ListActions'
import { AssetBadge } from './AssetsPage'
import { assetHistory, bookRows, type BookRow, monthsInUse } from './book'
import {
  type Asset, type AssetBooking, useAddAssetNote, useDeleteAssetNote, useAsset, useDeleteAsset, useLinkableBookings, useLinkSource, useUnlinkSource,
} from './queries'
import '../invoices/view.css'
import './assets.css'

/** One fixed asset (/assets/:assetId), after Moneybird's page: its values, its depreciations by year, and beside them its documents, state and history. */
export function AssetPage() {
  const { administrationId = '', assetId = '' } = useParams()
  const asset = useAsset(administrationId, assetId)
  if (asset.error instanceof ApiError && asset.error.status < 500) {
    return <NotFoundPage />
  }
  if (!asset.data) {
    return <Loading />
  }
  return <AssetView key={String(asset.data.id)} administrationId={administrationId} asset={asset.data} />
}

function AssetView({ administrationId, asset }: { administrationId: string; asset: Asset }) {
  const { t, i18n } = useTranslation()
  const administration = useAdministration(administrationId)
  const currency = administration.data?.currency ?? 'EUR'
  const money = (amount: string | number | null | undefined) => formatAmount(String(amount ?? '0'), currency, i18n.language)
  const base = `/${administrationId}/assets/${asset.id}`
  const plan = asset.value_change_plan
  const navigate = useNavigate()
  const remove = useDeleteAsset(administrationId, String(asset.id ?? ''))
  const [confirming, setConfirming] = useState(false)
  const summary: [string, string][] = [
    [t('assets.purchaseValue'), money(asset.purchase_value)],
    [t('assets.currentValue'), money(asset.current_value)],
    [t('assets.residualValue'), money(plan?.residual_value ?? 0)],
    [t('assets.monthlyDepreciation'), money(plan?.monthly_depreciation ?? 0)],
  ]

  return (
    <Page
      wide
      title={asset.name ?? ''}
      actions={
        <>
          <Link
            className="btn btn--secondary btn--icon"
            to={`/${administrationId}/report/journal_entries?ledger_account_id=${asset.ledger_account_id}`}
            aria-label={t('assets.journalEntries')}
            title={t('assets.journalEntries')}
          >
            <ListIcon kind="scale" />
          </Link>
          <Link className="btn btn--secondary btn--with-icon" to={`${base}/edit`}>
            <ListIcon kind="edit" />
            <span>{t('assets.edit')}</span>
          </Link>
          <MoreMenu items={[{ label: t('assets.delete'), danger: true, onSelect: () => setConfirming(true) }]} />
        </>
      }
    >
      <Problems messages={formErrors(remove.error, {}).general} />
      {confirming && (
        <ConfirmPanel
          message={t('assets.confirmDelete')}
          confirmLabel={t('assets.delete')}
          disabled={remove.isPending}
          onConfirm={() => remove.mutate(undefined, { onSuccess: () => navigate(`/${administrationId}/assets`) })}
          onCancel={() => setConfirming(false)}
        />
      )}
      <div className="invoice-view asset-view">
        <div className="asset-view__main">
          <section className="asset-summary" aria-label={t('assets.summary')}>
            {summary.map(([label, value]) => (
              <div key={label} className="asset-summary__item">
                <h4>{label}</h4>
                <p>{value}</p>
              </div>
            ))}
          </section>
          <DepreciationTable asset={asset} money={money} />
        </div>
        <AssetSidebar administrationId={administrationId} asset={asset} currency={currency} money={money} />
      </div>
    </Page>
  )
}

/** Moneybird's table of value changes: newest first, the years before this one folded into a total. */
function DepreciationTable({ asset, money }: { asset: Asset; money: (a: string | number) => string }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState<Set<string>>(new Set())
  const groups = bookRows(asset, new Date().getFullYear())
  const row = (r: BookRow) => (
    <tr key={r.key}>
      <td>{r.date}</td>
      <td>{t(`assets.types.${r.type}`, { defaultValue: r.type })}</td>
      <td className="document-table__amount">{money(r.depreciation)}</td>
      <td className="document-table__amount">{money(r.bookValue)}</td>
    </tr>
  )
  if (groups.length === 0) {
    return <p className="empty asset-view__empty">{t('assets.noValueChanges')}</p>
  }
  return (
    <div className="asset-table">
      <table className="data-table" aria-label={t('assets.valueChanges')}>
        <thead>
          <tr>
            <th>{t('assets.date')}</th>
            <th>{t('assets.depreciationType')}</th>
            <th className="document-table__amount">{t('assets.depreciation')}</th>
            <th className="document-table__amount">{t('assets.bookValue')}</th>
          </tr>
        </thead>
        <tbody>
          {groups.map((g) => g.total ? (
            <Fragment key={g.year}>
              <tr className="asset-table__year">
                <th scope="row">
                  <button
                    type="button"
                    className="asset-table__toggle"
                    aria-expanded={open.has(g.year)}
                    aria-label={t('assets.showYear', { year: g.year })}
                    onClick={() => setOpen((o) => { const n = new Set(o); if (n.has(g.year)) n.delete(g.year); else n.add(g.year); return n })}
                  >
                    <CaretIcon />
                    <span>{g.total.date}</span>
                  </button>
                </th>
                <td />
                <td className="document-table__amount">{money(g.total.depreciation)}</td>
                <td className="document-table__amount">{money(g.total.bookValue)}</td>
              </tr>
              {open.has(g.year) && g.rows.map(row)}
            </Fragment>
          ) : g.rows.map(row))}
        </tbody>
      </table>
    </div>
  )
}

function AssetSidebar({ administrationId, asset, currency, money }: { administrationId: string; asset: Asset; currency: string; money: (a: string | number) => string }) {
  const { t } = useTranslation()
  const accounts = useLedgerAccounts(administrationId).data ?? []
  const add = useAddAssetNote(administrationId, String(asset.id))
  const remove = useDeleteAssetNote(administrationId, String(asset.id))
  const years = asset.value_change_plan?.lifespan_in_years
  const months = monthsInUse(asset.purchase_date ?? '', new Date())
  const category = accounts.find((a) => a.id === asset.ledger_account_id)?.name
  return (
    <aside className="invoice-sidebar asset-sidebar" aria-label={t('assets.sidebar')}>
      <LinkedDocuments administrationId={administrationId} asset={asset} money={money} />
      <dl className="asset-facts">
        <div><dt>{t('assets.status')}</dt><dd><AssetBadge asset={asset} /></dd></div>
        <div><dt>{t('assets.commissioningDate')}</dt><dd>{asset.purchase_date}</dd></div>
        {years !== undefined && <div><dt>{t('assets.inUse')}</dt><dd>{t('assets.years', { count: years })}</dd></div>}
        <div><dt>{t('assets.operatingTime')}</dt><dd>{t('assets.months', { count: months })}</dd></div>
      </dl>
      {years !== undefined && years > 0 && <Timeline years={years} months={months} />}
      {category && (
        <dl className="asset-facts">
          <div><dt>{t('assets.category')}</dt><dd><span className="asset-category asset-category--full">{category}</span></dd></div>
        </dl>
      )}
      <NotesAndHistory administrationId={administrationId} items={assetHistory(asset)} currency={currency} add={add} remove={remove} />
    </aside>
  )
}

/** Moneybird's bar of the years in use, filled up to now. */
function Timeline({ years, months }: { years: number; months: number }) {
  const { t } = useTranslation()
  const whole = Math.max(1, Math.round(years))
  const done = Math.min(1, months / (whole * 12))
  return (
    <div className="asset-timeline" role="img" aria-label={t('assets.timeline', { months, years: whole })}>
      <div className="asset-timeline__bar">
        {Array.from({ length: whole }, (_, i) => (
          <span key={i} className="asset-timeline__step">
            <span className="asset-timeline__fill" style={{ width: `${Math.max(0, Math.min(1, done * whole - i)) * 100}%` }} />
          </span>
        ))}
        <span className="asset-timeline__now" style={{ left: `${done * 100}%` }} />
      </div>
      <div className="asset-timeline__labels" aria-hidden="true">
        {Array.from({ length: whole + 1 }, (_, i) => <span key={i}>{i}</span>)}
      </div>
    </div>
  )
}

/** The purchase value and the documents the asset was bought with, as Moneybird's linked entities. */
function LinkedDocuments({ administrationId, asset, money }: { administrationId: string; asset: Asset; money: (a: string | number) => string }) {
  const { t } = useTranslation()
  const [shown, setShown] = useState(false)
  const [picking, setPicking] = useState(false)
  const id = String(asset.id)
  const link = useLinkSource(administrationId, id)
  const unlink = useUnlinkSource(administrationId, id)
  const linkable = useLinkableBookings(administrationId, id, picking)
  // The contract adds the booking to Moneybird's source; the generated intersection keeps only Moneybird's fields on items.
  const sources = (asset.sources ?? []) as (NonNullable<Asset['sources']>[number] & { booking?: AssetBooking | null })[]
  const errors = formErrors(link.error ?? unlink.error, {})
  const documentPath = (b: AssetBooking) => b.document_type === 'GeneralJournalDocument'
    ? `/${administrationId}/documents/general_journal_documents/${b.document_id}`
    : `/${administrationId}/documents/${b.document_id}`
  const name = (b: AssetBooking) => b.description || b.reference || t(`assets.documentTypes.${b.document_type}`)
  return (
    <div className="asset-links">
      <div className="asset-links__row">
        <span>{t('assets.purchaseValue')}</span>
        <span>{money(asset.purchase_value ?? 0)}</span>
      </div>
      <button type="button" className="asset-links__row asset-links__button" aria-expanded={shown} onClick={() => setShown(!shown)}>
        <span>{t('assets.linkedDocuments', { count: sources.length })}</span>
        <CaretIcon />
      </button>
      {shown && (
        <ul className="asset-links__list" aria-label={t('assets.linkedDocumentsList')}>
          {sources.length === 0 && <li className="asset-links__empty">{t('assets.noLinkedDocuments')}</li>}
          {sources.map((s) => (
            <li key={s.id} className="asset-links__entity">
              {s.booking ? <Link to={documentPath(s.booking)}>{name(s.booking)}</Link> : <span>{t('assets.bookingGone')}</span>}
              {s.booking && <span>{money(s.booking.amount)}</span>}
              <button type="button" className="asset-links__unlink" disabled={unlink.isPending} onClick={() => unlink.mutate(String(s.id))}>
                {t('assets.unlink')}
              </button>
            </li>
          ))}
        </ul>
      )}
      <button type="button" className="asset-links__row asset-links__button" aria-expanded={picking} onClick={() => setPicking(!picking)}>
        <span>{t('assets.linkDocument')}</span>
        <ListIcon kind="plus" />
      </button>
      {picking && (
        <ul className="asset-links__list" aria-label={t('assets.linkableBookings')}>
          {!linkable.data ? <li className="asset-links__empty"><Loading /></li> : linkable.data.length === 0 && <li className="asset-links__empty">{t('assets.nothingToLink')}</li>}
          {(linkable.data ?? []).map((b) => (
            <li key={`${b.detail_id ?? ''}-${b.general_journal_document_entry_id ?? ''}`} className="asset-links__entity">
              <button
                type="button"
                className="asset-links__pick"
                disabled={link.isPending}
                onClick={() => link.mutate(b, { onSuccess: () => { setPicking(false); setShown(true) } })}
              >
                <span>{name(b)}</span>
                <span className="asset-links__meta">{b.date} · {money(b.amount)}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
      <Problems messages={errors.general} />
    </div>
  )
}
