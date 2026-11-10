import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { Problems } from '../auth/AuthLayout'
import { formErrors, FormNotice, FormSection, SelectInput } from '../forms/Form'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { can, useSupportAccess } from './queries'

const choices = [1, 7, 30] as const

/** Lets this installation's admins into the administration for a while. */
export function SupportAccessPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const administration = useAdministration(administrationId)
  const update = useSupportAccess(administrationId)
  const [days, setDays] = useState<number>(7)
  if (!administration.data) {
    return <Loading />
  }
  const a = administration.data
  const until = a.support_access_until && new Date(a.support_access_until) > new Date() ? new Date(a.support_access_until) : null
  const owner = can(a, 'ownership')
  const back = `/${administrationId}/settings/administration#privacy`
  const errors = formErrors(update.error, { until: t('management.duration') })
  const submit = (e: FormEvent) => {
    e.preventDefault()
    update.mutate(new Date(Date.now() + days * 24 * 3600 * 1000).toISOString())
  }
  return (
    <Page title={t('management.supportAccess')} subtitle={t('management.supportSubtitle')} back={{ to: back, label: t('management.back') }}>
      <form className="form-sheet" onSubmit={submit} noValidate>
        <FormNotice>
          {until
            ? t('management.supportActive', { date: until.toLocaleString(i18n.language, { dateStyle: 'long', timeStyle: 'short' }) })
            : t('management.supportOff')}
        </FormNotice>
        <Problems messages={errors.general} />
        {owner && (
          <>
            <FormSection>
              <SelectInput label={t('management.duration')} width="medium" value={String(days)} errors={errors.field('until')} onChange={(e) => setDays(Number(e.target.value))}>
                {choices.map((d) => <option key={d} value={d}>{t('management.days', { count: d })}</option>)}
              </SelectInput>
            </FormSection>
            <div className="form-sheet__actions">
              <button className="btn btn--primary" disabled={update.isPending}>{until ? t('management.extend') : t('management.grant')}</button>
              {until && <button type="button" className="btn btn--secondary" disabled={update.isPending} onClick={() => update.mutate(null)}>{t('management.endAccess')}</button>}
            </div>
          </>
        )}
      </form>
    </Page>
  )
}
