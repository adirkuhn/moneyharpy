import { type FormEvent, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router-dom'
import type { components } from '../api/schema'
import { useCurrentUser } from '../auth/queries'
import { manualPath, useCompanySearch, useOnboardingOptions } from './queries'
import { Wizard } from './Wizard'

type Company = components['schemas']['CompanyMatch']

const searchDelayMs = 300

export function CreateAdministrationPage() {
  const { t } = useTranslation()
  const me = useCurrentUser()
  const options = useOnboardingOptions()
  if (!options.data) {
    return null
  }
  const title = t('onboarding.welcome', { name: me.data?.name.split(' ')[0] ?? '' })
  const subtitle = t('onboarding.subtitle')

  if (!options.data.company_search_countries.includes('NL')) {
    return (
      <Wizard
        art
        title={title}
        subtitle={subtitle}
        footer={
          <>
            <Link className="btn btn--secondary" to="/administrations">{t('common.cancel')}</Link>
            <Link className="btn btn--primary" to={manualPath()}>{t('common.next')}</Link>
          </>
        }
      />
    )
  }
  return <CompanySearch title={title} subtitle={subtitle} />
}

function CompanySearch({ title, subtitle }: { title: string; subtitle: string }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [input, setInput] = useState('')
  const [term, setTerm] = useState('')
  const [selected, setSelected] = useState<Company | null>(null)
  const search = useCompanySearch(term)

  useEffect(() => {
    const timer = setTimeout(() => setTerm(input.trim()), searchDelayMs)
    return () => clearTimeout(timer)
  }, [input])
  useEffect(() => setSelected(null), [term])

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (selected) {
      navigate(manualPath(), { state: { company: selected } })
    }
  }

  return (
    <Wizard
      art
      title={title}
      subtitle={subtitle}
      onSubmit={submit}
      footer={
        <>
          <Link className="btn btn--secondary" to="/administrations">{t('common.cancel')}</Link>
          <button className="btn btn--primary" disabled={!selected}>{t('common.next')}</button>
        </>
      }
    >
      <label className="field">
        <span>{t('onboarding.companyQuestion')}</span>
        <input type="text" autoComplete="off" placeholder={t('onboarding.searchPlaceholder')} value={input} onChange={(e) => setInput(e.target.value)} />
      </label>
      <p className="hint">
        {t('onboarding.searchHelp')} <Link to={manualPath()}>{t('onboarding.notRegistered')}</Link>
      </p>
      {search.isError && <p role="alert" className="problems">{t('onboarding.searchFailed')}</p>}
      {search.data?.length === 0 && <p className="empty">{t('onboarding.noResults')}</p>}
      {search.data && search.data.length > 0 && (
        <div className="choices">
          {search.data.map((c) => (
            <label className="choice" key={c.chamber_of_commerce + c.address1}>
              <input type="radio" name="company" checked={selected === c} onChange={() => setSelected(c)} />
              <span className="choice__text">
                <strong>{c.name}</strong>
                <span className="hint">{t('onboarding.kvkNumber', { number: c.chamber_of_commerce })}</span>
                {(c.address1 || c.city) && <span className="hint">{[c.address1, c.city].filter(Boolean).join(', ')}</span>}
              </span>
            </label>
          ))}
        </div>
      )}
    </Wizard>
  )
}
