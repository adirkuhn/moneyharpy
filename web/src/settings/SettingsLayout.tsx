import { useTranslation } from 'react-i18next'
import { Link, Outlet, useLocation, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { ArrowLeftIcon, SectionIcon } from '../layout/icons'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Loading } from '../layout/Loading'

export const sections = ['company', 'invoices_and_estimates', 'bookkeeping', 'bank', 'administration', 'connections'] as const
export type Section = (typeof sections)[number]

/** The pages of a section, shown under it in the menu while it is open. */
const parts: Partial<Record<Section, { id: string; label: string }[]>> = {
  company: [
    { id: 'company-details', label: 'settings.details' },
    { id: 'sender-addresses', label: 'settings.senders' },
  ],
  invoices_and_estimates: [
    { id: 'document-styles', label: 'invoicing.parts.styles' },
    { id: 'workflows', label: 'invoicing.parts.workflows' },
    { id: 'numbering', label: 'invoicing.parts.numbering' },
    { id: 'peppol-ubl', label: 'invoicing.parts.peppol' },
    { id: 'custom-fields', label: 'invoicing.parts.customFields' },
    { id: 'forward-documents', label: 'invoicing.parts.forwarding' },
    { id: 'payment-methods', label: 'invoicing.parts.payments' },
    { id: 'payments-without-proof', label: 'invoicing.parts.withoutProof' },
  ],
  bookkeeping: [
    { id: 'declaration-period', label: 'bookkeeping.parts.periods' },
    { id: 'vat-rates', label: 'bookkeeping.parts.rates' },
    { id: 'categories', label: 'bookkeeping.parts.categories' },
    { id: 'chart-scheme', label: 'bookkeeping.parts.scheme' },
    { id: 'locked-period', label: 'bookkeeping.parts.lock' },
    { id: 'export', label: 'bookkeeping.parts.export' },
  ],
}

/** Pages outside /settings and the section they belong to. */
const pageSections: Record<string, Section> = {
  identities: 'company',
  document_styles: 'invoices_and_estimates',
  workflows: 'invoices_and_estimates',
  custom_fields: 'invoices_and_estimates',
  tax_rates: 'bookkeeping',
  ledger_accounts: 'bookkeeping',
  ledger_account_templates: 'bookkeeping',
}

/**
 * The settings area of one administration: a section menu beside the page.
 * Pages outside /settings, such as the identity forms, belong to a section.
 */
export function SettingsLayout() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const { pathname } = useLocation()
  const valid = /^\d{1,19}$/.test(administrationId)
  const administration = useAdministration(administrationId)

  if (!valid || (administration.error instanceof ApiError && administration.error.status < 500)) {
    return <NotFoundPage />
  }
  if (!administration.data) {
    return <Loading />
  }
  const base = `/${administrationId}`
  const [, , area, page] = pathname.split('/')
  const current: Section = area === 'settings' && sections.includes(page as Section) ? (page as Section) : (pageSections[area ?? ''] ?? 'company')
  const onSectionPage = area === 'settings' && page === current

  return (
    <div className="settings">
      <nav className="settings__nav" aria-label={t('settings.title')}>
        <Link className="settings__back" to={base}>
          <ArrowLeftIcon />
          {t('settings.back')}
        </Link>
        <p className="settings__heading">{t('settings.title')}</p>
        <ul>
          {sections.map((section) => (
            <li key={section}>
              <Link
                to={`${base}/settings/${section}`}
                className={section === current ? 'is-active' : undefined}
                aria-current={section === current ? (onSectionPage ? 'page' : 'true') : undefined}
              >
                <SectionIcon section={section} />
                {t(`settings.sections.${section}`)}
              </Link>
              {section === current && parts[section] && (
                <ul className="settings__parts">
                  {parts[section].map((part) => (
                    <li key={part.id}>
                      <Link to={`${base}/settings/${section}#${part.id}`}>{t(part.label)}</Link>
                    </li>
                  ))}
                </ul>
              )}
            </li>
          ))}
        </ul>
      </nav>
      <div className="settings__page">
        <Outlet />
      </div>
    </div>
  )
}
