import { type ReactNode, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { useCurrentUser } from '../auth/queries'
import { formatAmount } from '../banking/queries'
import { ListIcon, TileIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { Menu } from '../layout/Menu'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { can } from '../management/queries'
import { overdueStates, unpaidStates } from '../invoices/queries'
import { type OverviewItem, type OverviewTally, partOfDay, rememberAdministration, useOverview } from './queries'

/** The administration's home page: what waits in each part of the books and what was processed last. */
export function OverviewPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const valid = /^\d{1,19}$/.test(administrationId)
  const administration = useAdministration(administrationId)
  const overview = useOverview(administrationId)
  const me = useCurrentUser()

  useEffect(() => {
    if (administration.data) {
      rememberAdministration(administration.data.id)
    }
  }, [administration.data])

  if (!valid || (administration.error instanceof ApiError && administration.error.status < 500)) {
    return <NotFoundPage />
  }
  if (!administration.data) {
    return <Loading />
  }
  const a = administration.data
  const firstName = me.data?.name.trim().split(/\s+/)[0] ?? ''
  const greeting = t(`overview.${partOfDay(new Date(), a.time_zone)}`, { name: firstName })
  const money = (amount: string) => formatAmount(amount, a.currency, i18n.language)
  const count = (v: OverviewTally | undefined) => v?.count.toLocaleString(i18n.language)
  const bank = can(a, 'bank')
  const sales = can(a, 'sales_invoices')
  const documents = can(a, 'documents')
  const settings = can(a, 'settings')
  const base = `/${a.id}`

  return (
    <Page
      title={greeting}
      actions={(sales || documents || settings) && (
        <Menu label={<><ListIcon kind="plus" />{t('overview.add')}</>} buttonClassName="btn btn--accent">
          <nav aria-label={t('overview.addMenu')}>
            {(sales || documents) && (
              <ul className="menu__list">
                {sales && <li><Link to={`${base}/sales_invoices/new`}>{t('overview.addInvoice')}</Link></li>}
                {sales && <li><Link to={`${base}/external_sales_invoices/new`}>{t('overview.addExternal')}</Link></li>}
                {sales && <li><Link to={`${base}/recurring_sales_invoices/new`}>{t('overview.addRecurring')}</Link></li>}
                {documents && <li><Link to={`${base}/documents/general_journal_documents/new`}>{t('overview.addJournal')}</Link></li>}
                <li><Link to={`${base}/contacts/new`}>{t('overview.addContact')}</Link></li>
              </ul>
            )}
            {(sales || documents) && settings && <hr />}
            {settings && (
              <ul className="menu__list">
                <li><Link to={`${base}/financial_accounts/new`}>{t('overview.addAccount')}</Link></li>
                <li><Link to={`${base}/booking_rules/new`}>{t('overview.addRule')}</Link></li>
                <li><Link to={`${base}/ledger_accounts/new`}>{t('overview.addCategory')}</Link></li>
              </ul>
            )}
          </nav>
        </Menu>
      )}
    >
      {overview.isError && <p className="alert" role="alert">{t('overview.loadError')}</p>}
      {!bank && !sales && !documents && <p className="empty">{t('overview.nothing')}</p>}
      <div className="overview">
        {bank && (
          <Block title={t('overview.bank')}>
            <Tile icon="balance" title={t('overview.balance')} to={`${base}/settings/bank`} detail={overview.data?.bank && money(overview.data.bank.balance)} />
            <Tile icon="attention" urgent title={t('overview.toLink')} to={`${base}/financial_mutations?state=unprocessed&period=all`} detail={overview.data?.bank?.unprocessed_count.toLocaleString(i18n.language)} />
          </Block>
        )}
        {sales && (
          <Block title={t('overview.salesInvoices')}>
            <Tile icon="in" title={t('overview.toReceive')} count={count(overview.data?.sales?.to_receive)} to={`${base}/sales_invoices?filter=${encodeURIComponent(`state:${unpaidStates}`)}`} detail={overview.data?.sales && money(overview.data.sales.to_receive.amount)} />
            <Tile icon="attention" urgent title={t('overview.toRemind')} to={`${base}/sales_invoices?filter=${encodeURIComponent(`state:${overdueStates}`)}`} detail={count(overview.data?.sales?.to_remind)} />
          </Block>
        )}
        {documents && (
          <Block title={t('overview.incoming')}>
            <Later icon="out" title={t('overview.toPay')} />
            <Later icon="attention" title={t('overview.toProcess')} />
          </Block>
        )}
      </div>
      {bank && overview.data && <Recent items={overview.data.recently_processed} />}
    </Page>
  )
}

function Block({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="overview__block" aria-label={title}>
      <h2 className="overview__title">{title}</h2>
      {children}
    </section>
  )
}

type TileProps = { icon: Parameters<typeof TileIcon>[0]['kind']; title: string; count?: string; detail?: string; to: string; urgent?: boolean }

/** A summary that opens the list behind it; like Moneybird a count follows the title, the amount below. */
function Tile({ icon, title, count, detail, to, urgent }: TileProps) {
  return (
    <Link className="tile" to={to}>
      <span className={urgent ? 'tile__icon tile__icon--urgent' : 'tile__icon'}><TileIcon kind={icon} /></span>
      <span className="tile__title">{title}{count !== undefined && <span className="tile__count"> — {count}</span>}</span>
      <span className="tile__detail">{detail ?? '…'}</span>
    </Link>
  )
}

/** A tile for a part of MoneyHarpy that is not built yet. */
function Later({ icon, title }: Pick<TileProps, 'icon' | 'title'>) {
  const { t } = useTranslation()
  return (
    <div className="tile tile--later" title={t('overview.laterHint')}>
      <span className="tile__icon"><TileIcon kind={icon} /></span>
      <span className="tile__title">{title}</span>
      <span className="tile__detail">{t('overview.later')}</span>
    </div>
  )
}

function Recent({ items }: { items: OverviewItem[] }) {
  const { t, i18n } = useTranslation()
  const date = (d: string) => new Date(`${d}T00:00:00`).toLocaleDateString(i18n.language, { month: 'short', day: 'numeric' })
  return (
    <section className="recent" aria-label={t('overview.recent')}>
      <h2 className="recent__heading">{t('overview.recent')}</h2>
      {items.length === 0 ? (
        <p className="empty">{t('overview.noRecent')}</p>
      ) : (
        <ul className="recent__list">
          {items.map((it) => (
            <li key={`${it.type}-${it.id}`}>
              <Link className="recent__row" to={it.uri}>
                <span className="recent__kind" title={t(`overview.types.${it.type}`)}><TileIcon kind={Number(it.amount) < 0 ? 'out' : 'in'} /></span>
                <span className="recent__title">{it.title || t('bank.noDescription')}</span>
                <span className="recent__state">{t(`overview.states.${it.state}`, { defaultValue: it.state })}</span>
                <span className="recent__description">{it.description}</span>
                <span className="recent__amount">{formatAmount(it.amount, it.currency, i18n.language)}</span>
                <span className="recent__date">{date(it.date)}</span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
