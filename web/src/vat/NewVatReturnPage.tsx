import { type FormEvent, type ReactNode, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { formErrors } from '../forms/Form'
import { InfoIcon, ListIcon, WarningIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { BoxTable, CopyTable, useEuros } from './BoxTable'
import { IcpTable } from './IcpTable'
import { periodName } from './period'
import { type IcpKind, periodParam, useSubmitVatReturn, useVatPeriods, useVatPreview, type VatKind, vatKinds, type VatPreview } from './queries'
import './vat.css'

const loginURL = 'https://www.belastingdienst.nl/wps/wcm/connect/nl/ondernemers/content/inloggen-voor-ondernemers'
const helpURL = 'https://www.belastingdienst.nl/wps/wcm/connect/nl/btw/content/ik-moet-btw-aangifte-doen-hoe-vul-ik-die-in'

const steps = ['type', 'period', 'check', 'submit'] as const
type Step = (typeof steps)[number]

type StepProps = {
  title: string
  done: boolean
  current: boolean
  /** What a done step chose, which opens it again. */
  value?: string
  onEdit?: () => void
  children?: ReactNode
}

/** One step of Moneybird's declare card: its mark and title, what was chosen at the right, and its fields while it is the current one. */
function StepRow({ title, done, current, value, onEdit, children }: StepProps) {
  const id = useId()
  // As on Moneybird, a step shows once it is reached.
  if (!done && !current) {
    return null
  }
  return (
    <section className={`vat-step${current ? ' is-current' : ''}`} aria-labelledby={id}>
      <header className="vat-step__head">
        <span className={`vat-step__mark${done ? ' is-done' : ''}`} aria-hidden="true">{done && <ListIcon kind="check" />}</span>
        <h2 id={id} className="vat-step__title">{title}</h2>
        {done && value && onEdit && (
          <button type="button" className="vat-step__edit" onClick={onEdit}>
            <ListIcon kind="edit" />
            <span>{value}</span>
          </button>
        )}
      </header>
      {current && <div className="vat-step__body">{children}</div>}
    </section>
  )
}

/** The bar under a step with its centred button, as Moneybird's. */
const Footer = ({ children }: { children: ReactNode }) => <div className="vat-step__footer">{children}</div>

/**
 * Moneybird's "Declare new VAT Return" (/declarations/new), filed by hand:
 * one card whose steps open in turn: the kind, the period, the checks with
 * the amounts, and the amounts to type over at the tax authority before
 * marking it filed.
 */
export function NewVatReturnPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const administration = useAdministration(administrationId)
  const [step, setStep] = useState<Step>('type')
  const [kind, setKind] = useState<VatKind>('vat_return')
  const [period, setPeriod] = useState('')
  // Each kind's periods: a kind without any cannot be chosen, as on Moneybird.
  const byKind = { vat_return: useVatPeriods(administrationId, 'vat_return'), supplement: useVatPeriods(administrationId, 'supplement'), icp_declaration: useVatPeriods(administrationId, 'icp_declaration') }
  const periods = byKind[kind]
  const loaded = vatKinds.every((k) => byKind[k].data !== undefined)
  const unavailable = vatKinds.filter((k) => byKind[k].data?.length === 0)
  const shownKind = unavailable.includes(kind) ? (vatKinds.find((k) => !unavailable.includes(k)) ?? kind) : kind
  const preview = useVatPreview(administrationId, kind, step === 'check' || step === 'submit' ? period : undefined)

  if (administration.error instanceof ApiError && administration.error.status < 500) {
    return <NotFoundPage />
  }
  if (!administration.data) {
    return <Loading />
  }
  const list = `/${administrationId}/declarations`
  const options = periods.data ?? []
  const chosen = options.find((p) => periodParam(p) === period) ?? options[0]
  const name = chosen ? periodName(chosen.period_from, chosen.period_to, t, i18n.language) : ''
  const at = steps.indexOf(step)
  const row = (s: Step) => ({ done: steps.indexOf(s) < at, current: s === step, onEdit: () => setStep(s) })

  return (
    <Page wide title={t('vat.new.title')} back={{ to: list, label: t('vat.backToList') }}>
      <div className="vat-wizard">
        <StepRow title={t('vat.new.type')} value={t(`vat.kinds.${kind}`)} {...row('type')}>
          {!loaded ? <Loading /> : (
            <form onSubmit={(e) => { e.preventDefault(); setKind(shownKind); setPeriod(''); setStep('period') }}>
              <fieldset className="vat-kinds">
                <legend className="sr-only">{t('vat.new.type')}</legend>
                {vatKinds.map((k) => {
                  const off = unavailable.includes(k)
                  return (
                    <label key={k} className={`vat-kind${k === shownKind && !off ? ' is-checked' : ''}${off ? ' is-disabled' : ''}`}>
                      <span className="vat-kind__title">{t(`vat.kinds.${k}`)}</span>
                      <input type="radio" name="kind" value={k} checked={k === shownKind && !off} disabled={off} onChange={() => setKind(k)} />
                      <span className="vat-kind__text">{t(`vat.new.kindText.${k}`)}</span>
                    </label>
                  )
                })}
              </fieldset>
              {unavailable.map((k) => (
                <p key={k} className="vat-info vat-info--below"><InfoIcon /><span>{t(`vat.new.unavailable.${k}`)}</span></p>
              ))}
              <Footer><button className="btn btn--primary" disabled={unavailable.length === vatKinds.length}>{t('vat.new.next')}</button></Footer>
            </form>
          )}
        </StepRow>

        <StepRow title={t('vat.new.period')} value={name} {...row('period')}>
          {!periods.data ? <Loading /> : (
            <form onSubmit={(e) => { e.preventDefault(); if (chosen) { setPeriod(periodParam(chosen)); setStep('check') } }}>
              {kind !== 'supplement' && chosen && (
                <p className="vat-info">
                  <InfoIcon />
                  <span>
                    {t('vat.new.periodSetting', { period: t(`vat.granularity.${chosen.granularity}`) })}{' '}
                    <Link to={`/${administrationId}/settings/bookkeeping`}>{t('vat.new.periodSettingLink')}</Link>
                  </span>
                </p>
              )}
              {options.length === 0 ? (
                <p className="vat-info"><InfoIcon /><span>{t(`vat.new.noPeriods.${kind}`)}</span></p>
              ) : (
                <label className="vat-field">
                  <span className="vat-field__label">{t('vat.new.period')}</span>
                  <select value={chosen ? periodParam(chosen) : ''} onChange={(e) => setPeriod(e.target.value)}>
                    {options.map((p) => <option key={periodParam(p)} value={periodParam(p)}>{periodName(p.period_from, p.period_to, t, i18n.language)}</option>)}
                  </select>
                </label>
              )}
              <Footer><button className="btn btn--primary" disabled={!chosen}>{t('vat.new.next')}</button></Footer>
            </form>
          )}
        </StepRow>

        <StepRow title={t('vat.new.check')} value={t('vat.new.checkAgain')} {...row('check')}>
          {!preview.data ? (preview.error ? <Problems messages={formErrors(preview.error, {}).general} /> : <Loading />) : (
            <>
              <button type="button" className="vat-step__edit vat-step__reload" onClick={() => void preview.refetch()}>
                <ListIcon kind="repeat" />
                <span>{t('vat.new.reload')}</span>
              </button>
              <Checks administrationId={administrationId} preview={preview.data} />
              {kind === 'icp_declaration'
                ? <IcpTable lines={preview.data.icp_lines} label={t('vat.icp.title')} />
                : <BoxTable figures={preview.data} periodName={name} label={t('vat.boxesOf', { period: name })} />}
              <Footer><button type="button" className="btn btn--primary" onClick={() => setStep('submit')}>{t('vat.new.next')}</button></Footer>
            </>
          )}
        </StepRow>

        <StepRow title={t('vat.new.submit')} {...row('submit')}>
          {preview.data && (
            <Submit administrationId={administrationId} companyName={administration.data.name ?? ''} kind={kind} period={period} name={name} preview={preview.data} />
          )}
        </StepRow>
      </div>
    </Page>
  )
}

/** What may be missing from the books, each a heading with what was found under it, and the checks that passed. */
function Checks({ administrationId, preview }: { administrationId: string; preview: VatPreview }) {
  const { t, i18n } = useTranslation()
  const c = preview.checks
  const money = (v: string) => new Intl.NumberFormat(i18n.language, { style: 'currency', currency: 'EUR' }).format(Number(v))
  const base = `/${administrationId}`
  const all = [
    { failed: c.open_transactions > 0, title: t('vat.checks.open.title'), text: <Link to={`${base}/financial_mutations?state=unprocessed`}>{t('vat.checks.open.text', { count: c.open_transactions })}</Link>, passed: t('vat.checks.open.passed') },
    { failed: Number(c.untaxed_purchases) !== 0, title: t('vat.checks.untaxed.title'), text: t('vat.checks.untaxed.text', { amount: money(c.untaxed_purchases) }), passed: t('vat.checks.untaxed.passed') },
    { failed: c.bookings_without_vat > 0, title: t('vat.checks.withoutVat.title'), text: t('vat.checks.withoutVat.text', { count: c.bookings_without_vat }), passed: t('vat.checks.withoutVat.passed') },
    { failed: c.unprocessed_documents > 0, title: t('vat.checks.documents.title'), text: <Link to={`${base}/documents`}>{t('vat.checks.documents.text', { count: c.unprocessed_documents })}</Link>, passed: t('vat.checks.documents.passed') },
  ]
  const passed = all.filter((x) => !x.failed)
  return (
    <ul className="vat-checks" aria-label={t('vat.new.check')}>
      {all.filter((x) => x.failed).map((x) => (
        <li key={x.title} className="vat-check">
          <span className="vat-check__head is-warning"><WarningIcon /><strong>{x.title}</strong></span>
          <span className="vat-check__text">{x.text}</span>
        </li>
      ))}
      {passed.length > 0 && (
        <li className="vat-check">
          <details>
            <summary className="vat-check__head is-passed"><ListIcon kind="check" /><strong>{t('vat.checks.passed', { count: passed.length })}</strong></summary>
            {passed.map((x) => <span key={x.title} className="vat-check__text">{x.passed}</span>)}
          </details>
        </li>
      )}
    </ul>
  )
}

type SubmitProps = { administrationId: string; companyName: string; kind: VatKind; period: string; name: string; preview: VatPreview }

/**
 * Moneybird's submit step, by hand: log in at the tax authority, type the
 * amounts over, file once the total matches; then confirm and mark it filed
 * here.
 */
function Submit({ administrationId, companyName, kind, period, name, preview }: SubmitProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const euros = useEuros()
  const submit = useSubmitVatReturn(administrationId)
  const [kinds, setKinds] = useState<Record<string, IcpKind>>({})
  const [confirmed, setConfirmed] = useState(false)
  const late = new Date().toISOString().slice(0, 10) > preview.deadline
  const send = (e: FormEvent) => {
    e.preventDefault()
    submit.mutate(
      { kind, period, ...(kind === 'icp_declaration' ? { icp_lines: preview.icp_lines.map((l) => ({ contact_id: l.contact_id, kind: kinds[l.contact_id] ?? l.kind })) } : {}) },
      { onSuccess: (saved) => navigate(`/${administrationId}/declarations/${saved.id}`) },
    )
  }
  return (
    <form onSubmit={send} aria-label={t('vat.new.submit')}>
      {late && <p className="vat-info is-warning" role="alert"><WarningIcon /><span>{t('vat.new.late', { date: preview.deadline })}</span></p>}
      <div className="vat-kinds vat-kinds--single">
        <label className="vat-kind is-checked">
          <span className="vat-kind__title">{t('vat.manual.title')}</span>
          <input type="radio" name="method" checked readOnly />
          <span className="vat-kind__text">{t('vat.manual.text')}</span>
        </label>
      </div>
      <ol className="vat-manual">
        <li>
          <span className="vat-manual__number" aria-hidden="true">1</span>
          <div>
            <strong>{t('vat.manual.login')}</strong>
            <p>{t('vat.manual.loginText', { period: name })}</p>
            <a className="btn btn--secondary" href={loginURL} target="_blank" rel="noopener noreferrer">{t('vat.manual.portal')}</a>
          </div>
        </li>
        <li>
          <span className="vat-manual__number" aria-hidden="true">2</span>
          <div>
            <strong>{t('vat.manual.fill')}</strong>
            <p>{t(kind === 'icp_declaration' ? 'vat.manual.fillIcp' : 'vat.manual.fillText')}</p>
            {kind === 'icp_declaration'
              ? <IcpTable lines={preview.icp_lines} label={t('vat.manual.amounts')} kinds={kinds} onKind={(id, k) => setKinds({ ...kinds, [id]: k })} />
              : <CopyTable figures={preview} label={t('vat.manual.amounts')} />}
          </div>
        </li>
        <li>
          <span className="vat-manual__number" aria-hidden="true">3</span>
          <div>
            <strong>{t('vat.manual.send')}</strong>
            <p>
              {kind === 'icp_declaration' ? t('vat.manual.sendIcp') : t('vat.manual.sendText', { total: euros(preview.total) })}{' '}
              <a href={helpURL} target="_blank" rel="noopener noreferrer">{t('vat.manual.help')}</a>
            </p>
          </div>
        </li>
      </ol>
      <div className="vat-field vat-field--top">
        <span className="vat-field__label">{t('vat.manual.confirmation')}</span>
        <label className="check">
          <input type="checkbox" checked={confirmed} onChange={(e) => setConfirmed(e.target.checked)} />
          <span>{t('vat.manual.confirm', { kind: t(`vat.kinds.${kind}`), name: companyName, period: name })}</span>
        </label>
      </div>
      <Problems messages={formErrors(submit.error, {}).general} />
      <Footer><button className="btn btn--primary" disabled={!confirmed || submit.isPending}>{t('vat.manual.markFiled')}</button></Footer>
    </form>
  )
}
