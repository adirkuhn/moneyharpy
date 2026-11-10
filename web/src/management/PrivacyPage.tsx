import { useTranslation } from 'react-i18next'
import { useParams } from 'react-router-dom'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'

const topics = ['data_processing', 'data_analysis'] as const

/** What happens with the administration's data. Fixed text: the instance operator handles the data, not MoneyHarpy. */
export function PrivacyPage() {
  const { t } = useTranslation()
  const { administrationId = '', topic = '' } = useParams()
  if (!(topics as readonly string[]).includes(topic)) {
    return <NotFoundPage />
  }
  const back = `/${administrationId}/settings/administration#privacy`
  return (
    <Page title={t(`management.privacy.${topic}.title`)} back={{ to: back, label: t('management.back') }}>
      <div className="prose">
        {(t(`management.privacy.${topic}.body`, { returnObjects: true }) as string[]).map((p) => <p key={p}>{p}</p>)}
      </div>
    </Page>
  )
}
