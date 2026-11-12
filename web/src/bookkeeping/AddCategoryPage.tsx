import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, Navigate, useParams } from 'react-router-dom'
import { ChevronIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { useCategoryFlow } from './flow'
import { useBookkeepingSettings, useTemplates } from './queries'
import './categories.css'

/**
 * Moneybird's Add category (/ledger_accounts/blueprints): the themes of the
 * administration's country to add categories from, or a single category.
 */
export function AddCategoryPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const flow = useCategoryFlow(administrationId)
  const templates = useTemplates(administrationId)
  const settings = useBookkeepingSettings(administrationId)
  const themesLabel = useId()
  if (!templates.data || !settings.data) {
    return <Loading />
  }
  const scheme = settings.data.chart_schemes.find((s) => s.id === settings.data.chart_scheme)
  const base = `/${administrationId}/ledger_accounts`
  const carried = flow.query.slice(1)
  return (
    <Page
      title={flow.forInvoice ? t('bookkeeping.addCategoryFor') : t('bookkeeping.addCategory')}
      back={{ to: flow.returnTo, label: flow.forInvoice ? t('bookkeeping.toInvoice') : t('bookkeeping.back') }}
    >
      <div className="category-add">
        <section className="category-add__item">
          <p id={themesLabel} className="category-add__text">{t('bookkeeping.themesIntro')}</p>
          {templates.data.length === 0 ? (
            <p className="empty">{t('bookkeeping.noTemplates')}</p>
          ) : (
            <ul className="category-links" aria-label={t('bookkeeping.themes')} aria-describedby={themesLabel}>
              {templates.data.map((tpl) => (
                <li key={tpl.key}>
                  <Link className="category-links__item" to={`${base}/blueprint_ledger_accounts/new?ledger_account_set=${encodeURIComponent(tpl.key)}${carried ? `&${carried}` : ''}`}>
                    <ThemeIcon theme={tpl.key} />
                    <span className="category-links__body">
                      <span className="category-links__title">
                        {tpl.name}
                        {tpl.applied && <span className="category-links__state">{t('bookkeeping.added')}</span>}
                      </span>
                      <span className="category-links__subtitle">{tpl.description}</span>
                    </span>
                    <ChevronIcon className="category-links__chevron" />
                  </Link>
                </li>
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

/** An icon for a theme, drawn for MoneyHarpy; themes without their own get a grid. */
function ThemeIcon({ theme }: { theme: string }) {
  const shapes: Record<string, React.ReactNode> = {
    company_car: <><path d="M5 16.5V12l2-5h10l2 5v4.5" /><path d="M3.500 12h17v4.500h-17Z" /><circle cx="7.500" cy="18" r="1.500" /><circle cx="16.500" cy="18" r="1.500" /></>,
    personnel: <><circle cx="9" cy="8.500" r="3" /><path d="M3.500 19c.6-3 2.800-4.800 5.500-4.800s4.900 1.800 5.500 4.800" /><path d="M15.500 5.800a3 3 0 0 1 0 5.400M17 14.500c1.900.5 3.100 2 3.500 4.500" /></>,
    inventory: <><path d="M3.500 8 12 3.500 20.500 8v8L12 20.500 3.500 16Z" /><path d="M3.500 8 12 12.500 20.500 8M12 12.500v8" /></>,
    loan: <><rect x="3.500" y="6.500" width="17" height="11" rx="1.500" /><circle cx="12" cy="12" r="2.500" /><path d="M6.500 9.500h.01M17.500 14.500h.01" strokeWidth={2.500} /></>,
    lease: <><path d="M4 20V9l8-5 8 5v11" /><path d="M9 20v-6h6v6" /></>,
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
