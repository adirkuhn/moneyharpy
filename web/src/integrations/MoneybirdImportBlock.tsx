import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Problems } from '../auth/AuthLayout'
import { FormRow, formErrors } from '../forms/Form'
import { Block } from '../settings/Block'
import './integrations.css'
import { type MoneybirdAdministration, type MoneybirdImport, useMoneybirdAdministrations, useMoneybirdImport, useMoneybirdImportProblems, useStartMoneybirdImport, useTryMoneybirdImportNow } from './queries'

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
      {current && <Progress administrationId={administrationId} job={current} />}
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
          // Once triggered it stays off: the import runs until it finishes, and the form makes way for its progress.
          <button className="btn btn--primary" disabled={!chosen || start.isPending || start.isSuccess}>
            {start.isPending || start.isSuccess ? t('mbImport.starting') : t('mbImport.start')}
          </button>
        ) : (
          <button className="btn btn--primary" disabled={!token.trim() || check.isPending}>{check.isPending ? t('mbImport.checking') : t('mbImport.check')}</button>
        )}
      </div>
    </form>
  )
}

/** How far a step is, 0 to 1: done, or its records so far of its total. */
function stepShare(s: MoneybirdImport['steps'][number]): number {
  if (s.done) {
    return 1
  }
  return s.total > 0 ? Math.min(1, (s.imported + s.skipped + s.failed) / s.total) : 0
}

/** How far the whole import is, in percent: every step counts the same. */
export function importPercent(job: MoneybirdImport): number {
  if (job.state === 'done') {
    return 100
  }
  if (job.steps.length === 0) {
    return 0
  }
  return Math.floor((job.steps.reduce((sum, s) => sum + stepShare(s), 0) / job.steps.length) * 100)
}

function Bar({ label, value }: { label: string; value: number }) {
  return (
    <span className="mb-import__bar" role="progressbar" aria-label={label} aria-valuemin={0} aria-valuemax={100} aria-valuenow={value}>
      <span className="mb-import__fill" style={{ width: `${value}%` }} />
    </span>
  )
}

/** The import's state, its progress over all steps and per step, the wait for Moneybird and what failed. */
/** Every record the import could not bring in, or brought in without part of it, a page at a time. */
function FailedRecords({ administrationId, failed }: { administrationId: string; failed: number }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [page, setPage] = useState(1)
  const list = useMoneybirdImportProblems(administrationId, page, open)
  const total = list.data?.total ?? 0
  const pages = Math.max(1, Math.ceil(total / 50))
  return (
    <details className="mb-import__problems" onToggle={(e) => setOpen((e.target as HTMLDetailsElement).open)}>
      <summary>{t('mbImport.problems', { count: failed })}</summary>
      {open && !list.data && <p className="hint">{t('mbImport.loadingProblems')}</p>}
      {list.data && (
        <>
          <table className="data-table mb-import__problem-list" aria-label={t('mbImport.problemList')}>
            <thead>
              <tr>
                <th scope="col">{t('mbImport.kind')}</th>
                <th scope="col">{t('mbImport.moneybirdId')}</th>
                <th scope="col">{t('mbImport.why')}</th>
              </tr>
            </thead>
            <tbody>
              {list.data.problems.map((p) => (
                <tr key={`${p.kind}:${p.source_id}:${p.message}`}>
                  <td>{t(`mbImport.kinds.${p.step}`, { defaultValue: p.step })}</td>
                  <td className="mb-import__id">{p.source_id === '0' ? '' : p.source_id}</td>
                  <td>{p.partial ? t('mbImport.partial', { why: p.message }) : p.message}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {pages > 1 && (
            <div className="mb-import__pages">
              <button type="button" className="btn btn--secondary btn--small" disabled={page <= 1} onClick={() => setPage(page - 1)}>{t('mbImport.previous')}</button>
              <span className="hint">{t('mbImport.page', { page, pages, total })}</span>
              <button type="button" className="btn btn--secondary btn--small" disabled={page >= pages} onClick={() => setPage(page + 1)}>{t('mbImport.next')}</button>
            </div>
          )}
        </>
      )}
    </details>
  )
}

function TryNow({ administrationId }: { administrationId: string }) {
  const { t } = useTranslation()
  const tryNow = useTryMoneybirdImportNow(administrationId)
  return (
    <button type="button" className="btn btn--secondary btn--small" disabled={tryNow.isPending} onClick={() => tryNow.mutate()}>
      {t('mbImport.tryNow')}
    </button>
  )
}

function Progress({ administrationId, job }: { administrationId: string; job: MoneybirdImport }) {
  const { t, i18n } = useTranslation()
  const time = (iso: string) => new Intl.DateTimeFormat(i18n.language, { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(iso))
  const percent = importPercent(job)
  const at = job.steps.findIndex((s) => !s.done)
  const current = job.state === 'running' && at >= 0 ? job.steps[at] : undefined
  return (
    <div className="card card-stack mb-import" aria-label={t('mbImport.progress')} role="region">
      <p className="mb-import__head">
        <span>{t('mbImport.from', { name: job.source_name })}</span>
        <span className={`badge badge--${job.state === 'done' ? 'paid' : job.state === 'failed' ? 'late' : 'open'}`}>{t(`mbImport.states.${job.state}`)}</span>
      </p>
      <div className="mb-import__overall">
        <Bar label={t('mbImport.overall')} value={percent} />
        <span className="mb-import__percent">{percent}%</span>
      </div>
      {current && (
        <p className="hint" aria-live="polite">
          {t('mbImport.at', { step: at + 1, of: job.steps.length, kind: t(`mbImport.kinds.${current.kind}`) })}
          {current.total > 0 && ` · ${t('mbImport.ofTotal', { count: current.imported + current.skipped + current.failed, total: current.total })}`}
        </p>
      )}
      {job.state === 'running' && job.waiting_until && (
        <p className="notice" role="status">{t('mbImport.waiting', { time: time(job.waiting_until) })}</p>
      )}
      {job.state === 'running' && job.next_attempt_at && (
        <div className="notice mb-import__queued" role="status">
          <span>{t('mbImport.queued', { time: time(job.next_attempt_at), error: job.error })}</span>
          <TryNow administrationId={administrationId} />
        </div>
      )}
      {job.state === 'failed' && job.error && <p className="form-input__error">{t('mbImport.failed', { error: job.error })}</p>}
      <table className="data-table mb-import__steps" aria-label={t('mbImport.steps')}>
        <thead>
          <tr>
            <th scope="col">{t('mbImport.kind')}</th>
            <th scope="col" className="mb-import__progress">{t('mbImport.progressColumn')}</th>
            <th scope="col" className="mb-import__count">{t('mbImport.imported')}</th>
            <th scope="col" className="mb-import__count">{t('mbImport.skipped')}</th>
            <th scope="col" className="mb-import__count">{t('mbImport.failedCount')}</th>
          </tr>
        </thead>
        <tbody>
          {job.steps.map((s) => (
            <tr key={s.kind} className={s === current ? 'is-current' : undefined}>
              <th scope="row">{t(`mbImport.kinds.${s.kind}`)}</th>
              <td className="mb-import__progress"><Bar label={t(`mbImport.kinds.${s.kind}`)} value={Math.floor(stepShare(s) * 100)} /></td>
              <td className="mb-import__count">{s.total > 0 ? t('mbImport.ofTotal', { count: s.imported, total: s.total }) : s.imported}</td>
              <td className="mb-import__count">{s.skipped}</td>
              <td className="mb-import__count">{s.failed}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {job.steps.some((s) => s.problems.length > 0) && (
        <FailedRecords administrationId={administrationId} failed={job.steps.reduce((n, s) => n + s.failed, 0)} />
      )}
    </div>
  )
}
