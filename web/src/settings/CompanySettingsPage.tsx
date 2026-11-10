import { type ReactNode, useEffect, useId } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation, useParams } from 'react-router-dom'
import { Page } from '../layout/Page'
import { countryName, formatIBAN } from './format'
import { type Identity, useIdentities } from './queries'

export function CompanySettingsPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const identities = useIdentities(administrationId)
  const main = identities.data?.find((i) => i.default)
  const senders = identities.data?.filter((i) => !i.default) ?? []
  const { hash } = useLocation()

  // The menu links to parts of this page; bring the part into view once loaded.
  useEffect(() => {
    if (hash && identities.data) {
      document.getElementById(hash.slice(1))?.scrollIntoView?.({ block: 'start' })
    }
  }, [hash, identities.data])

  return (
    <Page title={t('settings.sections.company')} subtitle={t('settings.companySubtitle')}>
      <Block id="company-details" title={t('settings.details')} subtitle={t('settings.detailsSubtitle')}>
        {main && <IdentityCard identity={main} editPath={`/${administrationId}/identities/edit_default`} />}
      </Block>
      <Block
        id="sender-addresses"
        title={t('settings.senders')}
        subtitle={t('settings.sendersSubtitle')}
        action={<Link className="btn btn--accent" to={`/${administrationId}/identities/new`}>{t('settings.add')}</Link>}
      >
        {identities.data && senders.length === 0 && <p className="empty">{t('settings.noSenders')}</p>}
        {senders.map((s) => <IdentityCard key={s.id} identity={s} editPath={`${s.uri}/edit`} />)}
      </Block>
    </Page>
  )
}

type BlockProps = { id: string; title: string; subtitle: string; action?: ReactNode; children: ReactNode }

function Block({ id, title, subtitle, action, children }: BlockProps) {
  const headingId = useId()
  return (
    <section id={id} className="settings-block" aria-labelledby={headingId}>
      <header className="settings-block__header">
        <div>
          <h2 id={headingId}>{title}</h2>
          <p className="hint">{subtitle}</p>
        </div>
        {action}
      </header>
      {children}
    </section>
  )
}

function IdentityCard({ identity: i, editPath }: { identity: Identity; editPath: string }) {
  const { t, i18n } = useTranslation()
  const lines = (...values: (string | null | undefined)[]) => values.filter(Boolean) as string[]
  const rows: [string, string[]][] = [
    [t('settings.legalEntityType'), lines(i.legal_entity_type)],
    [t('settings.address'), lines(i.address1, i.address2, [i.zipcode, i.city].filter(Boolean).join(' '), countryName(i18n.language, i.country))],
    [t('settings.email'), lines(i.email)],
    [t('settings.phone'), lines(i.phone)],
    [t('settings.coc'), lines(i.chamber_of_commerce)],
    [t('settings.vat'), lines(i.tax_number)],
    [t('settings.bankAccount'), lines(formatIBAN(i.bank_account_number), i.bank_account_bic, i.bank_account_name)],
  ]
  return (
    <div className="datalist">
      <div className="datalist__header">
        <p className="datalist__title">{i.company_name}</p>
        <Link className="btn btn--secondary" to={editPath} aria-label={i.default ? undefined : t('settings.editName', { name: i.company_name })}>
          {t('settings.edit')}
        </Link>
      </div>
      <dl>
        {rows.map(([label, values]) => (
          <div className="datalist__row" key={label}>
            <dt>{label}</dt>
            <dd>{values.length ? values.map((v) => <div key={v}>{v}</div>) : <><span className="datalist__empty" aria-hidden="true">-</span><span className="sr-only">{t('settings.notSet')}</span></>}</dd>
          </div>
        ))}
      </dl>
    </div>
  )
}
