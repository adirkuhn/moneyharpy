import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Problems } from '../auth/AuthLayout'
import { FormRow, formErrors } from '../forms/Form'
import { Block } from '../settings/Block'
import './integrations.css'
import { type MoneybirdAdministration, type MoneybirdImport, useMoneybirdAdministrations, useMoneybirdImport, useStartMoneybirdImport } from './queries'

/**
 * Settings > Integrations > Import from Moneybird: a personal API token, the
 * Moneybird administration to copy, and how the import goes. Only owners
 * see it, as an import replaces the categories and VAT rates.
 */
export function MoneybirdImportBlock({ administrationId }: { administrationId: string }) {
  const { t } = useTranslation()
  const job = useMoneybirdImport(administrationId)
  const [again, setAgain] = useState(false)
  const current = job.data
  const showForm = !current || (again && current.state !== 'running')
  return (
    <Block id="moneybird-import" title={t('mbImport.title')} subtitle={t('mbImport.subtitle')}>
      {current && <Progress job={current} />}
      {current && current.state !== 'running' && !again && (
        <button type="button" className="btn btn--secondary" onClick={() => setAgain(true)}>{t('mbImport.again')}</button>
      )}
      {job.isSuccess && showForm && (
        <StartForm administrationId={administrationId} source={current?.source_administration_id} onStarted={() => setAgain(false)} />
      )}
    </Block>
  )
}

function StartForm({ administrationId, source, onStarted }: { administrationId: string; source?: string; onStarted: () => void }) {
  const { t } = useTranslation()
  const check = useMoneybirdAdministrations(administrationId)
  const start = useStartMoneybirdImport(administrationId)
  const [token, setToken] = useState('')
  const [chosen, setChosen] = useState(source ?? '')
  const found: MoneybirdAdministration[] | undefined = check.data
  const labels = { token: t('mbImport.token'), administration_id: t('mbImport.administration') }
  const errors = formErrors(start.error ?? check.error, labels)

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!found) {
      check.mutate(token.trim(), { onSuccess: (list) => setChosen(source ?? (list.length === 1 ? (list[0]?.id ?? '') : '')) })
      return
    }
    start.mutate({ token: token.trim(), administration_id: chosen }, { onSuccess: onStarted })
  }

  return (
    <form className="card card-stack mb-import__form" onSubmit={submit} noValidate aria-label={t('mbImport.title')}>
      <p className="hint">{source ? t('mbImport.againHelp') : t('mbImport.help')}</p>
      <Problems messages={errors.general} />
      <FormRow label={labels.token} errors={errors.field('token')} help={t('mbImport.tokenHelp')}>
        {({ labelId: _, ...control }) => (
          <input
            {...control}
            type="password"
            autoComplete="off"
            maxLength={500}
            value={token}
            onChange={(e) => {
              setToken(e.target.value)
              check.reset()
            }}
          />
        )}
      </FormRow>
      {found && (
        <fieldset className="radio-list mb-import__sources">
          <legend className="form-input__label">{labels.administration_id}</legend>
          {found.length === 0 && <p className="hint">{t('mbImport.none')}</p>}
          {found.map((a) => (
            <label key={a.id} className="radio-list__item">
              <input type="radio" name="moneybird-administration" value={a.id} checked={chosen === a.id} disabled={!!source && source !== a.id} onChange={() => setChosen(a.id)} />
              <span>{a.name}</span>
            </label>
          ))}
          {errors.field('administration_id')?.map((m) => <p key={m} className="form-input__error">{m}</p>)}
        </fieldset>
      )}
      <div className="mb-import__actions">
        {found ? (
          <button className="btn btn--primary" disabled={!chosen || start.isPending}>{t('mbImport.start')}</button>
        ) : (
          <button className="btn btn--primary" disabled={!token.trim() || check.isPending}>{t('mbImport.check')}</button>
        )}
      </div>
    </form>
  )
}

/** The import's state, its steps with their counts, the wait for Moneybird and what failed. */
function Progress({ job }: { job: MoneybirdImport }) {
  const { t, i18n } = useTranslation()
  const time = (iso: string) => new Intl.DateTimeFormat(i18n.language, { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(iso))
  return (
    <div className="card card-stack mb-import" aria-label={t('mbImport.progress')} role="region">
      <p className="mb-import__head">
        <span>{t('mbImport.from', { name: job.source_name })}</span>
        <span className={`badge badge--${job.state === 'done' ? 'paid' : job.state === 'failed' ? 'late' : 'open'}`}>{t(`mbImport.states.${job.state}`)}</span>
      </p>
      {job.state === 'running' && job.waiting_until && (
        <p className="notice" role="status">{t('mbImport.waiting', { time: time(job.waiting_until) })}</p>
      )}
      {job.error && <p className={job.state === 'failed' ? 'form-input__error' : 'hint'}>{job.state === 'failed' ? t('mbImport.failed', { error: job.error }) : t('mbImport.retrying', { error: job.error })}</p>}
      <table className="data-table mb-import__steps" aria-label={t('mbImport.steps')}>
        <thead>
          <tr>
            <th scope="col">{t('mbImport.kind')}</th>
            <th scope="col" className="mb-import__count">{t('mbImport.imported')}</th>
            <th scope="col" className="mb-import__count">{t('mbImport.skipped')}</th>
            <th scope="col" className="mb-import__count">{t('mbImport.failedCount')}</th>
            <th scope="col">{t('mbImport.stepState')}</th>
          </tr>
        </thead>
        <tbody>
          {job.steps.map((s) => (
            <tr key={s.kind}>
              <th scope="row">{t(`mbImport.kinds.${s.kind}`)}</th>
              <td className="mb-import__count">{s.total > 0 ? t('mbImport.ofTotal', { count: s.imported, total: s.total }) : s.imported}</td>
              <td className="mb-import__count">{s.skipped}</td>
              <td className="mb-import__count">{s.failed}</td>
              <td>{s.done ? t('mbImport.stepDone') : job.state === 'running' ? t('mbImport.stepBusy') : t('mbImport.stepWaiting')}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {job.steps.some((s) => s.problems.length > 0) && (
        <details className="mb-import__problems">
          <summary>{t('mbImport.problems', { count: job.steps.reduce((n, s) => n + s.failed, 0) })}</summary>
          <ul>
            {job.steps.flatMap((s) => s.problems.map((p) => <li key={`${s.kind}:${p}`}>{p}</li>))}
          </ul>
        </details>
      )}
    </div>
  )
}
