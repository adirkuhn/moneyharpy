import { ActionLink } from '../layout/ActionLink'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { InfoIcon, ListIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { GettingStarted } from '../list/GettingStarted'
import { periodName } from './period'
import { useVatReturns, type VatReturn } from './queries'
import './vat.css'

/** How a return's state shows: to pay or receive is open, paid and submitted are done. */
export const stateLook: Record<string, string> = { to_pay: 'open', to_receive: 'open', paid: 'paid', submitted: 'paid' }

/**
 * Moneybird's VAT returns (/declarations): the returns, supplements and ICP
 * declarations filed, per year, with their state; and the way to declare a
 * new one.
 */
export function VatReturnsPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const administration = useAdministration(administrationId)
  const returns = useVatReturns(administrationId)
  const [info, setInfo] = useState(false)
  const infoId = useId()

  if (administration.error instanceof ApiError && administration.error.status < 500) {
    return <NotFoundPage />
  }
  if (!administration.data) {
    return <Loading />
  }
  const base = `/${administrationId}/declarations`
  const list = returns.data ?? []
  const years = [...new Set(list.map((r) => r.period_from.slice(0, 4)))]
  const shown = info || (returns.data !== undefined && list.length === 0)

  return (
    <Page
      wide
      title={t('vat.title')}
      actions={
        <div className="accounts-actions">
          <button
            type="button"
            className={`btn btn--secondary btn--icon${shown ? ' is-active' : ''}`}
            aria-label={t('list.moreInfo')}
            title={t('list.moreInfo')}
            aria-expanded={shown}
            aria-controls={infoId}
            onClick={() => setInfo(!info)}
          >
            <InfoIcon />
          </button>
          <ActionLink className="btn btn--accent btn--with-icon" to={`${base}/new`}>
            <ListIcon kind="plus" />
            <span>{t('vat.declare')}</span>
          </ActionLink>
        </div>
      }
    >
      {shown && (
        <GettingStarted
          id={infoId}
          title={t('vat.intro.title')}
          text={t('vat.intro.text')}
          steps={[
            { icon: 'scale', title: t('vat.intro.check'), text: t('vat.intro.checkText') },
            { icon: 'repeat', title: t('vat.intro.corrections'), text: t('vat.intro.correctionsText') },
            { icon: 'send', title: t('vat.intro.icp'), text: t('vat.intro.icpText') },
          ]}
        />
      )}
      {!returns.data ? (
        returns.error ? <p className="empty">{t('vat.loadError')}</p> : <Loading />
      ) : list.length === 0 ? (
        <p className="empty">{t('vat.empty')}</p>
      ) : (
        years.map((year) => (
          <section key={year} className="vat-year" aria-labelledby={`${infoId}-${year}`}>
            <h2 id={`${infoId}-${year}`} className="vat-year__title">{year}</h2>
            <YearTable returns={list.filter((r) => r.period_from.startsWith(year))} base={base} label={year} language={i18n.language} />
          </section>
        ))
      )}
    </Page>
  )
}

function YearTable({ returns, base, label, language }: { returns: VatReturn[]; base: string; label: string; language: string }) {
  const { t } = useTranslation()
  return (
    <table className="data-table vat-list" aria-label={t('vat.yearList', { year: label })}>
      <thead>
        <tr>
          <th scope="col">{t('vat.type')}</th>
          <th scope="col">{t('vat.period')}</th>
          <th scope="col">{t('vat.sentOn')}</th>
          <th scope="col">{t('vat.status')}</th>
        </tr>
      </thead>
      <tbody>
        {returns.map((r) => (
          <tr key={r.id}>
            <td><Link to={`${base}/${r.id}`}>{t(`vat.kinds.${r.kind}`)}</Link></td>
            <td>{periodName(r.period_from, r.period_to, t, language).replace(/\s\d{4}$/, '')}</td>
            <td>{r.sent_on}</td>
            <td className="document-table__state"><span className={`badge badge--${stateLook[r.state] ?? 'open'}`}>{t(`vat.states.${r.state}`)}</span></td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
