import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { Wizard } from './Wizard'
import { Loading } from '../layout/Loading'
import { PageIcon, type PageIconKind } from '../layout/pageIcons'

const actions = ['invoice', 'purchase', 'bank', 'open'] as const
type Action = (typeof actions)[number]
const icons: Record<Action, PageIconKind> = { invoice: 'invoice', purchase: 'receipt', bank: 'card', open: 'cabinet' }

export function FirstActionPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const navigate = useNavigate()
  const administration = useAdministration(administrationId)
  const [choice, setChoice] = useState<Action | null>(null)

  if (!administration.data) {
    return <Loading />
  }
  // Every choice opens the administration until its feature exists.
  const submit = (e: FormEvent) => {
    e.preventDefault()
    navigate(administration.data.uri)
  }

  return (
    <Wizard
      art
      languageSwitch={false}
      title={t('onboarding.firstTitle')}
      subtitle={t('onboarding.firstSubtitle')}
      onSubmit={submit}
      footer={<button className="btn btn--primary wizard__end" disabled={!choice}>{t('onboarding.getStarted')}</button>}
    >
      <div className="choices" role="radiogroup" aria-label={t('onboarding.firstTitle')}>
        {actions.map((action) => (
          <label className="choice choice--card" key={action}>
            <span className="choice__icon"><PageIcon kind={icons[action]} /></span>
            <input type="radio" name="first_action" checked={choice === action} onChange={() => setChoice(action)} />
            <span className="choice__text">
              <strong>{t(`onboarding.${action}`)}</strong>
              <span className="hint">{t(`onboarding.${action}Body`)}</span>
            </span>
          </label>
        ))}
      </div>
    </Wizard>
  )
}
