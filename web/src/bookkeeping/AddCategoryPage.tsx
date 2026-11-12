import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, Navigate, useParams } from 'react-router-dom'
import { ChevronIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { useIdentities } from '../settings/queries'
import { useCategoryFlow } from './flow'
import { type Template, useBookkeepingSettings, useTemplates } from './queries'
import './categories.css'

/**
 * Moneybird's Add category (/ledger_accounts/blueprints): the themes to add
 * categories from, determined by the company's legal form when it has one and
 * else by the administration's country, or a single category.
 */
export function AddCategoryPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const flow = useCategoryFlow(administrationId)
  const templates = useTemplates(administrationId)
  const settings = useBookkeepingSettings(administrationId)
  const identities = useIdentities(administrationId)
  const themesLabel = useId()
  if (!templates.data || !settings.data || identities.isPending) {
    return <Loading />
  }
  const scheme = settings.data.chart_schemes.find((s) => s.id === settings.data.chart_scheme)
  const company = identities.data?.find((i) => i.default) ?? identities.data?.[0]
  const legalForm = company?.legal_entity_type?.trim()
  const base = `/${administrationId}/ledger_accounts`
  const carried = flow.query.slice(1)
  return (
    <Page
      title={flow.forInvoice ? t('bookkeeping.addCategoryFor') : t('bookkeeping.addCategory')}
      back={{ to: flow.returnTo, label: flow.forInvoice ? t('bookkeeping.toInvoice') : t('bookkeeping.back') }}
    >
      <div className="category-add">
        <section className="category-add__item">
          <p id={themesLabel} className="category-add__text">
            {legalForm ? (
              <>
                {t('bookkeeping.legalFormIntro')} <Link className="category-add__legal-form" to={`/${administrationId}/identities/edit_default`}>{legalForm}</Link>
              </>
            ) : t('bookkeeping.themesIntro')}
          </p>
          {templates.data.length === 0 ? (
            <p className="empty">{t('bookkeeping.noTemplates')}</p>
          ) : (
            <ul className="category-links" aria-label={t('bookkeeping.themes')} aria-describedby={themesLabel}>
              {templates.data.map((tpl) => (
                <ThemeLink key={tpl.key} template={tpl} to={`${base}/blueprint_ledger_accounts/new?ledger_account_set=${encodeURIComponent(tpl.key)}${carried ? `&${carried}` : ''}`} />
              ))}
            </ul>
          )}
        </section>
        <section className="category-add__item">
          <p className="category-add__text">{scheme ? t('bookkeeping.singleIntroScheme', { scheme: scheme.name }) : t('bookkeeping.singleIntro')}</p>
          <ul className="category-links">
            <li>
              <Link className="category-links__item" to={`${base}/new${flow.query}`}>
                <ThemeIcon theme="single" />
                <span className="category-links__body">
                  <span className="category-links__title">{t('bookkeeping.single.title')}</span>
                </span>
                <ChevronIcon className="category-links__chevron" />
              </Link>
            </li>
          </ul>
        </section>
      </div>
    </Page>
  )
}

/** One theme: its icon, name and what it is for, and whether all of it is there. */
function ThemeLink({ template: tpl, to }: { template: Template; to: string }) {
  const { t } = useTranslation()
  const about = useId()
  return (
    <li>
      <Link className="category-links__item" to={to} aria-label={tpl.name} aria-describedby={about}>
        <ThemeIcon theme={tpl.key} />
        <span className="category-links__body">
          <span className="category-links__title">
            {tpl.name}
            {tpl.applied && <span className="category-links__state">{t('bookkeeping.added')}</span>}
          </span>
          <span id={about} className="category-links__subtitle">
            {tpl.applied && <span className="sr-only">{t('bookkeeping.added')}. </span>}
            {tpl.description}
          </span>
        </span>
        <ChevronIcon className="category-links__chevron" />
      </Link>
    </li>
  )
}

/** An icon for a theme, drawn for MoneyHarpy after the kind Moneybird shows; themes without their own get a grid. */
function ThemeIcon({ theme }: { theme: string }) {
  const calculator = <><rect x="5.500" y="3.500" width="13" height="17" rx="2" /><path d="M8.500 7h7v3h-7Z" /><path d="M9 13.500h.01M12 13.500h.01M15 13.500h.01M9 17h.01M12 17h.01M15 17h.01" strokeWidth={2.200} /></>
  const shapes: Record<string, React.ReactNode> = {
    revenue: <><rect x="2.500" y="6.500" width="19" height="11" rx="1.500" /><circle cx="12" cy="12" r="2.500" /><path d="M5.500 9.500h.01M18.500 14.500h.01" strokeWidth={2.500} /></>,
    direct_costs: calculator,
    other_costs_set: calculator,
    sales_set: <><path d="M4 10v4h3l6 4V6L7 10Z" /><path d="M16.500 9a4 4 0 0 1 0 6M7 14l1.500 5h2.500l-1-4" /></>,
    car_set: <><path d="M5 16.500V12l2-5h10l2 5v4.500" /><path d="M3.500 12h17v4.500h-17Z" /><circle cx="7.500" cy="18" r="1.500" /><circle cx="16.500" cy="18" r="1.500" /></>,
    housing: <><path d="M5 20.500V4.500h9v16M14 9.500h5v11" /><path d="M8 8h3M8 11.500h3M8 15h3M3.500 20.500h17" /></>,
    staff_set: <><circle cx="9" cy="8.500" r="3" /><path d="M3.500 19c.6-3 2.800-4.800 5.500-4.800s4.900 1.800 5.500 4.800" /><path d="M15.500 5.800a3 3 0 0 1 0 5.400M17 14.500c1.900.5 3.100 2 3.500 4.500" /></>,
    entrepreneur: <><circle cx="10" cy="8.500" r="3" /><path d="M4 19c.6-3 2.900-4.800 6-4.800 1.400 0 2.700.4 3.700 1.100" /><path d="M18 13.500v6M15 16.500h6" /></>,
    fixed_assets: <><path d="M3.500 20.500h17" /><path d="m4.500 16 5-5 3.500 3.500 6.500-7" /><path d="M15.500 7.500h4v4" /></>,
    single: <><rect x="4" y="4" width="16" height="16" rx="2.500" /><path d="M12 8.500v7M8.500 12h7" /></>,
  }
  const grid = <><rect x="4" y="4" width="6.500" height="6.500" rx="1" /><rect x="13.500" y="4" width="6.500" height="6.500" rx="1" /><rect x="4" y="13.500" width="6.500" height="6.500" rx="1" /><rect x="13.500" y="13.500" width="6.500" height="6.500" rx="1" /></>
  return (
    <svg className="category-links__icon" viewBox="0 0 24 24" width="1.25em" height="1.25em" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth={1.600} strokeLinecap="round" strokeLinejoin="round">
      {shapes[theme] ?? grid}
    </svg>
  )
}

/** The templates page that came before Add category, kept so its links still work. */
export function TemplatesRedirect() {
  const { administrationId = '' } = useParams()
  return <Navigate to={`/${administrationId}/ledger_accounts/blueprints`} replace />
}
